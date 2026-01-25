package utils

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetAntigravityDBPath returns the path to the state.vscdb file.
// It searches in standard VS Code / Antigravity locations.
func GetAntigravityDBPath() string {
	var configDir string
	home, _ := os.UserHomeDir()

	switch runtime.GOOS {
	case "windows":
		configDir = os.Getenv("APPDATA")
	case "darwin":
		configDir = filepath.Join(home, "Library", "Application Support")
	case "linux":
		configDir = filepath.Join(home, ".config")
	}

	// List of possible folder names to check, prioritized
	possibleFolders := []string{"Antigravity", "Code"}

	for _, folder := range possibleFolders {
		path := filepath.Join(configDir, folder, "User", "globalStorage", "state.vscdb")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Return a default path even if not found, preferring Antigravity
	return filepath.Join(configDir, "Antigravity", "User", "globalStorage", "state.vscdb")
}

// GetAntigravityPath returns the path to the IDE executable.
func GetAntigravityPath() string {
	// This function tries to find the executable path.
	// In a real scenario, this might need more robust lookup or configuration.
	// For now, checks common locations.

	switch runtime.GOOS {
	case "windows":
		// Check typical install locations for Antigravity or VS Code
		// TODO: Add more paths if needed or registry lookup
		candidates := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Antigravity", "Antigravity.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "Code.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Antigravity", "Antigravity.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "Code.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
		return "Antigravity.exe" // Fallback to PATH lookup name
	case "darwin":
		candidates := []string{
			"/Applications/Antigravity.app/Contents/MacOS/Electron", // Approximate
			"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
		return "code"
	default: // linux
		return "code" // Assuming 'code' is in PATH
	}
}
