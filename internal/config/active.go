package config

import (
	"os"
	"path/filepath"
	"strings"
)

func getActiveProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".antigravity-cli", "active_profile")
}

// GetActiveProfileName reads the active profile name from disk.
func GetActiveProfileName() string {
	path := getActiveProfilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SetActiveProfileName writes the active profile name to disk.
func SetActiveProfileName(name string) error {
	path := getActiveProfilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(name), 0644)
}
