package config

import (
	"antigravity-cli/internal/security"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type SensitiveData struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	ExpiryTimestamp int64  `json:"expiry_timestamp"`
}

type Profile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`

	// SensitiveData fields are not stored directly in the main JSON
	AccessToken     string `json:"-"`
	RefreshToken    string `json:"-"`
	ExpiryTimestamp int64  `json:"-"`

	// EncryptedBlob stores the encrypted SensitiveData
	EncryptedBlob []byte `json:"encrypted_blob"`

	// Network settings
	ProxyScheme string `json:"proxy_scheme"` // socks5, http
	ProxyHost   string `json:"proxy_host"`
	ProxyPort   int    `json:"proxy_port"`
	ProxyUser   string `json:"proxy_user"`
	ProxyPass   string `json:"proxy_pass"`

	UseSystemTunnel bool `json:"use_system_tunnel"`
}

type Store struct {
	path     string
	Profiles map[string]Profile `json:"profiles"`
	mu       sync.RWMutex
}

func NewStore(path string) *Store {
	return &Store{
		path:     path,
		Profiles: make(map[string]Profile),
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &s.Profiles); err != nil {
		return err
	}

	// Decrypt sensitive data for all profiles
	for name, p := range s.Profiles {
		if len(p.EncryptedBlob) > 0 {
			decrypted, err := security.Decrypt(p.EncryptedBlob)
			if err != nil {
				continue
			}

			var sensitive SensitiveData
			if err := json.Unmarshal(decrypted, &sensitive); err == nil {
				p.AccessToken = sensitive.AccessToken
				p.RefreshToken = sensitive.RefreshToken
				p.ExpiryTimestamp = sensitive.ExpiryTimestamp
				s.Profiles[name] = p
			}
		}
	}

	return nil
}

func (s *Store) AddProfile(p Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Profiles[p.Name] = p
	return s.saveNoLock()
}

func (s *Store) saveNoLock() error {
	profilesToSave := make(map[string]Profile)

	for name, p := range s.Profiles {
		sensitive := SensitiveData{
			AccessToken:     p.AccessToken,
			RefreshToken:    p.RefreshToken,
			ExpiryTimestamp: p.ExpiryTimestamp,
		}
		
		jsonData, err := json.Marshal(sensitive)
		if err != nil {
			return err
		}

		encrypted, err := security.Encrypt(jsonData)
		if err != nil {
			return err
		}

		p.EncryptedBlob = encrypted
		profilesToSave[name] = p
	}

	data, err := json.MarshalIndent(profilesToSave, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveNoLock()
}

func (s *Store) GetProfile(name string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.Profiles[name]
	return p, ok
}
