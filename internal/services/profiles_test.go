package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveKeepsPasswordOutOfFile(t *testing.T) {
	secrets := &MemSecrets{}
	path := filepath.Join(t.TempDir(), "profiles.json")
	s := newProfileServiceAt(path, secrets)

	p, err := s.Save(Profile{Name: "dev", Broker: "tcp://localhost:1883", Password: "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.ClientID == "" {
		t.Errorf("ids not generated: %+v", p)
	}
	if p.Password != "" || !p.HasPassword {
		t.Errorf("returned profile leaks or misreports password: %+v", p)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "hunter2") {
		t.Fatalf("password written to disk:\n%s", data)
	}
	if got, _ := secrets.Get(p.ID); got != "hunter2" {
		t.Errorf("secret store has %q", got)
	}
	if l := s.List(); len(l) != 1 || l[0].Password != "" {
		t.Errorf("List leaks password: %+v", l)
	}

	// Update with a blank password keeps the stored one.
	p.Name = "renamed"
	p, err = s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasPassword {
		t.Error("HasPassword lost on update")
	}
	if got, _ := secrets.Get(p.ID); got != "hunter2" {
		t.Errorf("secret changed on blank update: %q", got)
	}
	if _, pw, err := s.withSecret(p.ID); err != nil || pw != "hunter2" {
		t.Errorf("withSecret = %q, %v", pw, err)
	}
}

func TestSaveValidatesBroker(t *testing.T) {
	s := newProfileServiceAt(filepath.Join(t.TempDir(), "p.json"), &MemSecrets{})
	for _, b := range []string{"", "localhost:1883", "http://x", "tcp://"} {
		if _, err := s.Save(Profile{Broker: b}); err == nil {
			t.Errorf("broker %q should be rejected", b)
		}
	}
	if _, err := s.Save(Profile{ID: "missing", Broker: "tcp://x:1"}); err == nil {
		t.Error("update of unknown id should fail")
	}
}

func TestLoadMigratesPlaintextPasswords(t *testing.T) {
	secrets := &MemSecrets{}
	path := filepath.Join(t.TempDir(), "profiles.json")
	legacy := []map[string]any{{"id": "abc", "name": "old", "broker": "tcp://x:1883", "password": "plain"}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	s := newProfileServiceAt(path, secrets)
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get("abc")
	if err != nil || p.Password != "" || !p.HasPassword {
		t.Fatalf("migrated profile = %+v, %v", p, err)
	}
	if got, _ := secrets.Get("abc"); got != "plain" {
		t.Errorf("secret not migrated: %q", got)
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "plain") {
		t.Fatalf("plaintext password still on disk:\n%s", after)
	}
}

func TestDeleteNotifiesAndRemovesSecret(t *testing.T) {
	secrets := &MemSecrets{}
	s := newProfileServiceAt(filepath.Join(t.TempDir(), "p.json"), secrets)
	var changes []ProfileChange
	s.onChange = func(c ProfileChange) { changes = append(changes, c) }

	p, _ := s.Save(Profile{Broker: "tcp://x:1", Password: "pw"})
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatal("delete must be idempotent")
	}
	if len(changes) != 2 || changes[0].Deleted || !changes[1].Deleted || changes[1].Profile.ID != p.ID {
		t.Errorf("changes = %+v", changes)
	}
	if got, _ := secrets.Get(p.ID); got != "" {
		t.Error("secret survived delete")
	}
	if len(s.List()) != 0 {
		t.Error("profile survived delete")
	}
}
