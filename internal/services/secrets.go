package services

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// SecretStore keeps broker passwords out of profiles.json. Keys are profile
// IDs. Get returns "" (not an error) when nothing is stored.
type SecretStore interface {
	Get(id string) (string, error)
	Set(id, secret string) error
	Delete(id string) error
}

const keyringService = "mqttc"

// KeyringSecrets stores secrets in the OS keychain (macOS Keychain, Windows
// Credential Manager, Secret Service on Linux).
type KeyringSecrets struct{}

func (KeyringSecrets) Get(id string) (string, error) {
	s, err := keyring.Get(keyringService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return s, err
}

func (KeyringSecrets) Set(id, secret string) error { return keyring.Set(keyringService, id, secret) }

func (KeyringSecrets) Delete(id string) error {
	err := keyring.Delete(keyringService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
