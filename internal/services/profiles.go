package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"mqttc/internal/mqtt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Profile is a stored broker connection. Password is write-only: the UI sends
// it on Save and it goes straight to the SecretStore; List/Get never return
// it. HasPassword tells the UI whether one is stored.
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Broker      string `json:"broker"` // tcp://host:1883
	ClientID    string `json:"clientId"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"` // write-only, never persisted to disk
	HasPassword bool   `json:"hasPassword"`
	CleanStart  bool   `json:"cleanStart"`
	Insecure    bool   `json:"insecure"` // skip TLS verify (dev brokers)
}

// ProfileChange is delivered to the onChange listener after the store was
// updated. Deleted profiles carry only their ID.
type ProfileChange struct {
	Profile Profile
	Deleted bool
}

type ProfileService struct {
	secrets  SecretStore
	onChange func(ProfileChange) // set once at startup; called outside the lock

	mu   sync.RWMutex
	path string
	list []Profile
}

func NewProfileService(secrets SecretStore) *ProfileService {
	return &ProfileService{secrets: secrets}
}

// newProfileServiceAt is NewProfileService with an explicit file, for tests.
func newProfileServiceAt(path string, secrets SecretStore) *ProfileService {
	return &ProfileService{secrets: secrets, path: path}
}

func (s *ProfileService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if s.path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(dir, "mqttc")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		s.path = filepath.Join(dir, "profiles.json")
	}
	return s.load()
}

func (s *ProfileService) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &s.list); err != nil {
		return fmt.Errorf("profiles.json: %w", err)
	}
	// A password in the file (hand-edited, or written by an older build) is
	// treated like any inbound password: moved to the keychain and blanked.
	moved := false
	for i := range s.list {
		if s.list[i].Password != "" {
			if err := s.storeSecret(&s.list[i]); err != nil {
				return err
			}
			moved = true
		}
	}
	if moved {
		return s.saveLocked()
	}
	return nil
}

// storeSecret moves a non-empty Password into the SecretStore.
func (s *ProfileService) storeSecret(p *Profile) error {
	if p.Password == "" {
		return nil
	}
	if err := s.secrets.Set(p.ID, p.Password); err != nil {
		return fmt.Errorf("storing password for %s: %w", p.ID, err)
	}
	p.Password, p.HasPassword = "", true
	return nil
}

func (s *ProfileService) saveLocked() error {
	data, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *ProfileService) notify(ch ProfileChange) {
	if s.onChange != nil {
		s.onChange(ch)
	}
}

// indexOf returns the position of id in list, or -1. Caller holds mu.
func (s *ProfileService) indexOf(id string) int {
	return slices.IndexFunc(s.list, func(p Profile) bool { return p.ID == id })
}

func (s *ProfileService) List() []Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.list)
}

func (s *ProfileService) Get(id string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i := s.indexOf(id); i >= 0 {
		return s.list[i], nil
	}
	return Profile{}, fmt.Errorf("profile %q not found", id)
}

// withSecret is Get plus the stored password, for the session manager only.
// It is unexported on purpose so Wails never exposes it to the frontend.
func (s *ProfileService) withSecret(id string) (Profile, string, error) {
	p, err := s.Get(id)
	if err != nil {
		return Profile{}, "", err
	}
	pw, err := s.secret(p)
	return p, pw, err
}

// secret returns p's stored password, skipping the keychain round trip when
// none is stored.
func (s *ProfileService) secret(p Profile) (string, error) {
	if !p.HasPassword {
		return "", nil
	}
	pw, err := s.secrets.Get(p.ID)
	if err != nil {
		return "", fmt.Errorf("reading password for %s: %w", p.Name, err)
	}
	return pw, nil
}

// Save creates (empty ID) or updates. An empty Password on update keeps the
// stored one. Returns the stored profile (without the password).
func (s *ProfileService) Save(p Profile) (Profile, error) {
	if _, _, err := mqtt.ParseBroker(p.Broker); err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	stored, err := s.upsertLocked(p)
	s.mu.Unlock()
	if err != nil {
		return Profile{}, err
	}
	s.notify(ProfileChange{Profile: stored})
	return stored, nil
}

func (s *ProfileService) upsertLocked(p Profile) (Profile, error) {
	if p.ClientID == "" {
		p.ClientID = "mqttc-" + randHex(4)
	}
	idx := -1
	if p.ID == "" {
		p.ID = randHex(8)
		p.HasPassword = false
	} else {
		if idx = s.indexOf(p.ID); idx < 0 {
			return Profile{}, fmt.Errorf("profile %q not found", p.ID)
		}
		p.HasPassword = s.list[idx].HasPassword
	}
	if err := s.storeSecret(&p); err != nil {
		return Profile{}, err
	}
	if idx < 0 {
		s.list = append(s.list, p)
	} else {
		s.list[idx] = p
	}
	return p, s.saveLocked()
}

func (s *ProfileService) Delete(id string) error {
	s.mu.Lock()
	idx := s.indexOf(id)
	if idx < 0 {
		s.mu.Unlock()
		return nil
	}
	s.list = slices.Delete(s.list, idx, idx+1)
	err := s.secrets.Delete(id)
	if err == nil {
		err = s.saveLocked()
	}
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("deleting profile: %w", err)
	}
	s.notify(ProfileChange{Profile: Profile{ID: id}, Deleted: true})
	return nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
