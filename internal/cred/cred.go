// Package cred loads and stores local Shared Memory device credentials.
package cred

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvHome   = "SHARED_MEMORY_HOME"
	EnvAPIURL         = "SHARED_MEMORY_API_URL"
	EnvAPIKey         = "SHARED_MEMORY_API_KEY"
	EnvConnectionID   = "SHARED_MEMORY_CONNECTION_ID"

	DefaultAPIURL = "http://127.0.0.1:18080"
	fileName      = "credentials.json"
)

var ErrNotFound = errors.New("credentials not found")

// Credentials is the on-disk device login state (mode 0600).
type Credentials struct {
	APIKey   string `json:"api_key"`
	APIURL   string `json:"api_url"`
	DeviceID     string `json:"device_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// Home returns ~/.shared-memory or SHARED_MEMORY_HOME.
func Home() (string, error) {
	if v := os.Getenv(EnvHome); v != "" {
		return v, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".shared-memory"), nil
}

func Path() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fileName), nil
}

func SpoolDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "spool"), nil
}

func EnsureHome() error {
	home, err := Home()
	if err != nil {
		return err
	}
	return os.MkdirAll(home, 0o700)
}

func Save(c Credentials) error {
	if err := EnsureHome(); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func Load() (Credentials, error) {
	path, err := Path()
	if err != nil {
		return Credentials{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return Credentials{}, fmt.Errorf("credentials: %w", err)
	}
	return c, nil
}

// Resolve merges env overrides on top of the credentials file.
// Env-only is allowed when both URL and key are set.
func Resolve() (Credentials, error) {
	c, err := Load()
	if err != nil && !IsNotFound(err) {
		return Credentials{}, err
	}
	if IsNotFound(err) {
		c = Credentials{}
	}
	if v := os.Getenv(EnvAPIURL); v != "" {
		c.APIURL = v
	}
	if v := os.Getenv(EnvAPIKey); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv(EnvConnectionID); v != "" {
		c.ConnectionID = strings.TrimSpace(v)
	}
	if c.APIURL == "" {
		c.APIURL = DefaultAPIURL
	}
	if c.APIKey == "" {
		return Credentials{}, ErrNotFound
	}
	return c, nil
}

func Delete() error {
	path, err := Path()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
