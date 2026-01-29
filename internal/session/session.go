package session

import (
	"antigravity-cli/internal/auth"
	"antigravity-cli/internal/config"
	"antigravity-cli/internal/injection"
	"antigravity-cli/internal/tunnel"
	"antigravity-cli/internal/utils"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/net/proxy"
)

// Session manages the active tunnel connection and profile
// Session manages the active tunnel connection and profile
type Session struct {
	mu          sync.Mutex
	singBoxCmd  *exec.Cmd
	singBoxExec string
	lockedPort  int
	profile     config.Profile
	configPath  string
	refreshQuit chan struct{}
	ideCmd      *exec.Cmd // Handle to the IDE process
}

// NewSession creates a new session manager
func NewSession() *Session {
	return &Session{}
}

// Start initializes a session with the given profile
func (s *Session) Start(profile config.Profile, port int, cmd *exec.Cmd, singBoxExec, configPath string, ideCmd *exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.singBoxCmd = cmd
	s.singBoxExec = singBoxExec
	s.lockedPort = port
	s.profile = profile
	s.configPath = configPath
	s.ideCmd = ideCmd
	
	// Start background refresh monitor
	s.refreshQuit = make(chan struct{})
	go s.monitorTokenExpiry()
	
	// Start IDE monitor
	if s.ideCmd != nil {
		go s.monitorIDE()
	}
}

// ProfileName returns the current profile name
func (s *Session) ProfileName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profile.Name
}

// LockedPort returns the locked port
func (s *Session) LockedPort() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lockedPort
}

// IsActive returns true if the session is active
func (s *Session) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.singBoxCmd != nil && s.singBoxCmd.Process != nil
}

// SwitchProfile atomically switches to a new profile
func (s *Session) SwitchProfile(newProfile config.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Println("\n🔄 Switching profile...")

	// 1. KILL IDE first (so it releases DB lock)
	fmt.Println("⏹️ Stopping IDE...")
	utils.KillAntigravity()
	time.Sleep(1 * time.Second)

	// 2. STOP old sing-box
	fmt.Println("⏹️ Stopping old tunnel...")
	if s.singBoxCmd != nil && s.singBoxCmd.Process != nil {
		s.singBoxCmd.Process.Kill()
		s.singBoxCmd.Wait()
	}
	if s.configPath != "" {
		os.Remove(s.configPath)
	}

	// Give port time to be released
	time.Sleep(200 * time.Millisecond)

	// 3. START new sing-box on the SAME port
	fmt.Println("▶️ Starting new tunnel...")
	proxyCfg := tunnel.ProxyConfig{
		ListenPort:      s.lockedPort, // Same port!
		ProxyType:       newProfile.ProxyScheme,
		ProxyHost:       newProfile.ProxyHost,
		ProxyPort:       newProfile.ProxyPort,
		ProxyUser:       newProfile.ProxyUser,
		ProxyPass:       newProfile.ProxyPass,
		UseSystemTunnel: newProfile.UseSystemTunnel,
	}

	configPath, err := tunnel.GenerateConfig(proxyCfg)
	if err != nil {
		return fmt.Errorf("failed to generate config: %v", err)
	}

	newCmd := exec.Command(s.singBoxExec, "run", "-c", configPath)
	newCmd.Stdout = os.Stdout
	newCmd.Stderr = os.Stderr

	if err := newCmd.Start(); err != nil {
		os.Remove(configPath)
		return fmt.Errorf("failed to start sing-box: %v", err)
	}

	// Wait for sing-box to start
	time.Sleep(500 * time.Millisecond)

	// 4. VERIFY: Check new tunnel works
	fmt.Println("🔍 Verifying new tunnel...")
	if !verifyConnection(s.lockedPort) {
		newCmd.Process.Kill()
		os.Remove(configPath)
		return fmt.Errorf("new tunnel verification failed")
	}
	fmt.Println("✅ New tunnel verified!")

	// 4.5. TOKEN REFRESH: Check if token needs refresh
	now := time.Now().Unix()
	expiresIn5Min := now + 5*60
	
	if newProfile.ExpiryTimestamp < expiresIn5Min {
		fmt.Println("🔄 Token expired or expiring soon, refreshing...")
		
		tokenResp, err := auth.RefreshAccessTokenViaProxy(newProfile.RefreshToken, s.lockedPort)
		if err != nil {
			fmt.Printf("⚠️ Token refresh failed: %v\n", err)
		} else {
			newProfile.AccessToken = tokenResp.AccessToken
			newProfile.ExpiryTimestamp = time.Now().Unix() + tokenResp.ExpiresIn
			fmt.Println("✅ Token refreshed!")
			
			// Save updated profile to store
			home, _ := os.UserHomeDir()
			store := config.NewStore(filepath.Join(home, ".antigravity-cli", "profiles.json"))
			if err := store.Load(); err == nil {
				store.Profiles[newProfile.Name] = newProfile
				store.Save()
			}
		}
	}

	// 5. CLEANUP + LOGIN: Clear old credentials then inject new
	fmt.Println("🚪 Clearing old credentials...")
	ClearIDECredentials()
	fmt.Println("🔑 Injecting new credentials...")
	dbPath := utils.GetAntigravityDBPath()
	if err := injection.InjectIdentity(dbPath, newProfile.AccessToken, newProfile.RefreshToken, newProfile.Email, newProfile.Name); err != nil {
		fmt.Printf("⚠️ Warning: injection failed: %v\n", err)
	}

	// 6. Start IDE with new proxy port
	fmt.Println("🚀 Starting IDE...")
	idePath := utils.GetAntigravityPath()
	ideCmd, err := utils.StartAntigravity(idePath, s.lockedPort)
	if err != nil {
		fmt.Printf("⚠️ Warning: failed to start IDE: %v\n", err)
	}

	// Update session state
	s.singBoxCmd = newCmd
	s.configPath = configPath
	s.profile = newProfile // Updated to set full profile
	s.ideCmd = ideCmd
	config.SetActiveProfileName(newProfile.Name)
	
	// Restart background monitor
	s.refreshQuit = make(chan struct{})
	go s.monitorTokenExpiry()
	if s.ideCmd != nil {
		go s.monitorIDE()
	}

	fmt.Printf("✅ Switched to profile: %s\n", newProfile.Name)
	return nil
}

// Stop terminates the session
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop background monitor
	if s.refreshQuit != nil {
		close(s.refreshQuit)
		s.refreshQuit = nil
	}

	if s.singBoxCmd != nil && s.singBoxCmd.Process != nil {
		s.singBoxCmd.Process.Kill()
		s.singBoxCmd.Wait()
	}
	if s.configPath != "" {
		os.Remove(s.configPath)
	}

	// Terminate IDE if still running
	if s.ideCmd != nil && s.ideCmd.Process != nil {
		s.ideCmd.Process.Kill() 
	}
}

// monitorIDE waits for the IDE process to exit
func (s *Session) monitorIDE() {
	if s.ideCmd == nil {
		return
	}
	// Wait for process to exit
	s.ideCmd.Wait()

	// Check if session is still active (not manually stopped)
	s.mu.Lock()
	isActive := s.singBoxCmd != nil
	s.mu.Unlock()

	if isActive {
		fmt.Println("\n⚠️ IDE closed. Cleaning up session...")
		// Use a separate goroutine or call Stop directly?
		// Stop() acquires lock, so it's safe.
		// Also we want to clear credentials.
		// Since this might overlap with manual stop, we should be careful.
		
		ClearIDECredentials() // Clear credentials immediately
		s.Stop() // Stop tunnel
		
		// If we are in TUI, this might print over it. 
		// Ideally we should signal the main loop to exit, but os.Exit is a harsh but effective way ensuring cleanup if we can't communicate back easily.
		// However, returning to menu is better. 
		// For now, let's just ensure clean state.
		fmt.Println("✅ Session cleanup complete. Press Enter to return to menu...")
	}
}

// ShowStatus displays current session status
func (s *Session) ShowStatus() {
	s.mu.Lock()
	port := s.lockedPort
	name := s.profile.Name
	s.mu.Unlock()

	fmt.Printf("\n📊 Session Status\n")
	fmt.Printf("   Profile: %s\n", name)
	fmt.Printf("   Port: %d\n", port)

	// Check public IP through proxy
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, proxy.Direct)
	if err != nil {
		fmt.Printf("   Public IP: Error creating dialer\n")
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{Dial: dialer.Dial},
		Timeout:   10 * time.Second,
	}

	resp, err := httpClient.Get("http://ip-api.com/line/?fields=query,country")
	if err != nil {
		fmt.Printf("   Public IP: Connection failed\n")
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	fmt.Printf("   Public IP: %s", string(buf[:n]))
}

// monitorTokenExpiry checks token expiry every minute
func (s *Session) monitorTokenExpiry() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.refreshQuit:
			return
		case <-ticker.C:
			s.checkAndRefresh()
		}
	}
}

// checkAndRefresh refreshes token if expiring soon
func (s *Session) checkAndRefresh() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we have a valid profile and running proxy
	if s.singBoxCmd == nil || s.singBoxCmd.Process == nil {
		return
	}

	now := time.Now().Unix()
	expiresIn5Min := now + 5*60

	if s.profile.ExpiryTimestamp < expiresIn5Min {
		// Need refresh
		// Note: We are holding the lock here, but RefreshAccessTokenViaProxy does network IO.
		// However, since it's a background task and only runs once an hour, holding the lock briefly is acceptable
		// to ensure consistency. If it blocks too long, UI might freeze if it calls methods needing lock.
		// Better to copy needed data and unlock, then refresh, then re-lock to save.
		
		refreshToken := s.profile.RefreshToken
		proxyPort := s.lockedPort
		profileName := s.profile.Name
		
		// Unlock for network request
		s.mu.Unlock()
		
		tokenResp, err := auth.RefreshAccessTokenViaProxy(refreshToken, proxyPort)
		
		// Re-lock to save
		s.mu.Lock()
		
		if err != nil {
			// Just log to debug file if available, or ignore. 
			// We can't easily print to stdout as it might interfere with TUI.
			return
		}

		// Update profile in session
		// Check if profile is still the same (user might have switched while we were refreshing)
		if s.profile.Name == profileName {
			s.profile.AccessToken = tokenResp.AccessToken
			s.profile.ExpiryTimestamp = time.Now().Unix() + tokenResp.ExpiresIn
			
			// Save to disk
			home, _ := os.UserHomeDir()
			store := config.NewStore(filepath.Join(home, ".antigravity-cli", "profiles.json"))
			if err := store.Load(); err == nil {
				store.Profiles[profileName] = s.profile
				store.Save()
			}
		}
	}
}

// blockIDE blocks IDE network access via Windows Firewall
func blockIDE() {
	if runtime.GOOS != "windows" {
		return
	}
	idePath := utils.GetAntigravityPath()
	exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=AntigravityBlock", "dir=out", "action=block",
		"program="+idePath).Run()
}

// unblockIDE removes the firewall block
func unblockIDE() {
	if runtime.GOOS != "windows" {
		return
	}
	exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name=AntigravityBlock").Run()
}

// ClearIDECredentials simulates a real "Sign Out" by matching what the app does:
// 1. Delete antigravity.profileUrl
// 2. Set antigravityAuthStatus to "null" (string)
// 3. Reset jetskiStateSync.agentManagerInitState to minimal protobuf "mgEA"
func ClearIDECredentials() error {
	dbPath := utils.GetAntigravityDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// 1. Delete profile avatar
	db.Exec("DELETE FROM ItemTable WHERE key = ?", "antigravity.profileUrl")
	
	// 2. Set auth status to "null" string (not NULL!)
	db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", "null", "antigravityAuthStatus")
	
	// 3. Reset protobuf to empty state "mgEA" (base64 of minimal protobuf)
	db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", "mgEA", "jetskiStateSync.agentManagerInitState")
	
	// 4. Also delete google.antigravity cache
	db.Exec("DELETE FROM ItemTable WHERE key = ?", "google.antigravity")

	return nil
}

// verifyConnection checks if the tunnel is working
func verifyConnection(port int) bool {
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, proxy.Direct)
	if err != nil {
		return false
	}

	httpClient := &http.Client{
		Transport: &http.Transport{Dial: dialer.Dial},
		Timeout:   15 * time.Second,
	}

	resp, err := httpClient.Get("http://clients3.google.com/generate_204")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// FindFreePort finds an available port in the given range
func FindFreePort(min, max int) int {
	for port := min; port <= max; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			return port
		}
	}
	return min
}

// GetSingBoxPath finds the sing-box executable
func GetSingBoxPath() (string, error) {
	// Try alongside executable
	ex, err := os.Executable()
	if err == nil {
		exDir := filepath.Dir(ex)
		localSingBox := filepath.Join(exDir, "sing-box.exe")
		if _, err := os.Stat(localSingBox); err == nil {
			return localSingBox, nil
		}
	}

	// Try CWD
	cwd, _ := os.Getwd()
	localSingBox := filepath.Join(cwd, "sing-box.exe")
	if _, err := os.Stat(localSingBox); err == nil {
		return localSingBox, nil
	}

	// Try PATH
	path, err := exec.LookPath("sing-box")
	if err == nil {
		absPath, _ := filepath.Abs(path)
		return absPath, nil
	}

	return "", fmt.Errorf("sing-box.exe not found")
}
