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
	// We need to iterate and update the map because Profile is a value type in the map
	for name, p := range s.Profiles {
		if len(p.EncryptedBlob) > 0 {
			decrypted, err := security.Decrypt(p.EncryptedBlob)
			if err != nil {
				// Log error but maybe continue? Or fail?
				// For now, let's just create an empty SensitiveData or return error.
				// Returning error might lock the user out if one profile is corrupted.
				// Let's print to stderr or just ignore (fields will be empty).
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
	defer s.mu.Unlock() // Use defer for safety
	
	// Ensure ID is set
	if p.ID == "" {
		// p.ID = uuid.NewString() // Need uuid? Or just use Name? User didn't specify.
		// Let's leave ID empty or set it to Name if missing for now, or assume caller sets it.
	}

	s.Profiles[p.Name] = p
	// We call Save(), which handles encryption
	// Unlock is deferred, but Save acquires RLock. RLock inside Lock? 
	// Save uses RLock. We have Lock. RLocking while holding Lock is fine? No, it might deadlock or be undefined depending on mutex implementation.
	// `sync.RWMutex`: "If a goroutine holds a RWMutex for reading and another goroutine might call Lock, no goroutine should expect to be able to acquire a read lock until the initial read lock is released."
	// Actually: "If a goroutine holds a RWMutex for writing, it is not allowed to call RLock." -> Deadlock!
	
	// So we cannot call s.Save() inside AddProfile if s.Save() locks everything.
	// We need an internal save helper or unlock before saving.
	
	return s.saveNoLock()
}

// saveNoLock saves without locking, assumes caller holds the lock (Read or Write? Save needs to read profiles).
// The caller of saveNoLock (AddProfile) holds a Write Lock.
// saveNoLock needs to read profiles. Since we have a Write Lock, we can read.
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

// Public Save method (if needed externally)
func (s *Store) Save() error {
	s.mu.Lock() // Use Lock because we might be updating EncryptedBlobs in a way (conceptually we are snapshotting)
	defer s.mu.Unlock()
	return s.saveNoLock()
}


func (s *Store) GetProfile(name string) (Profile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.Profiles[name]
	// p is already decrypted during Load(). 
	// Wait, Load() runs once. What if we add a profile? AddProfile updates s.Profiles[p.Name] = p.
	// If p passed to AddProfile has plain tokens (which it likely does), then s.Profiles has plain tokens.
	// Save() encrypts them for disk.
	// So GetProfile just returns what's in memory.
	return p, ok
}
