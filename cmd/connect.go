package cmd

import (
	"antigravity-cli/internal/auth"
	"antigravity-cli/internal/config"
	"antigravity-cli/internal/injection"
	"antigravity-cli/internal/session"
	"antigravity-cli/internal/tunnel"
	"antigravity-cli/internal/utils"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/proxy"
)

// activeSession holds the current session (used by session menu)
var activeSession *session.Session

// RunConnect - это публичная функция, которую мы будем вызывать из меню
func RunConnect(profileName string) error {
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, "antigravity_debug.log")
	
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err == nil {
		l := log.New(f, "CONNECT: ", log.LstdFlags)
		l.Printf("Starting connection for %s", profileName)
		defer f.Close()
	}

	fmt.Printf("Connecting to profile: %s\n", profileName)

	// 1. Загрузка профиля
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	storePath := filepath.Join(homeDir, ".antigravity-cli", "profiles.json")
	
	store := config.NewStore(storePath)
	if err := store.Load(); err != nil {
		return fmt.Errorf("Failed to load profiles: %v", err)
	}

	profile, ok := store.GetProfile(profileName)
	if !ok {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	if profile.AccessToken == "" || profile.AccessToken == "pending_auth" {
		return fmt.Errorf("profile has no tokens. Run 'antigravity login %s'", profileName)
	}

	// 2. Поиск порта
	fmt.Println("DEBUG: Finding free port...")
	localPort := session.FindFreePort(10000, 20000)
	fmt.Printf("DEBUG: Selected local port: %d\n", localPort)

	// 3. Генерация конфига
	fmt.Println("DEBUG: Generating sing-box config...")
	proxyCfg := tunnel.ProxyConfig{
		ListenPort:      localPort,
		ProxyType:       profile.ProxyScheme,
		ProxyHost:       profile.ProxyHost,
		ProxyPort:       profile.ProxyPort,
		ProxyUser:       profile.ProxyUser,
		ProxyPass:       profile.ProxyPass,
		UseSystemTunnel: profile.UseSystemTunnel,
	}
	configPath, err := tunnel.GenerateConfig(proxyCfg)
	if err != nil {
		return fmt.Errorf("failed to generate config: %v", err)
	}

	// 4. Поиск sing-box.exe
	singBoxExec, err := session.GetSingBoxPath()
	if err != nil {
		os.Remove(configPath)
		return fmt.Errorf("❌ CRITICAL: %v", err)
	}

	// Запуск sing-box
	fmt.Printf("DEBUG: Launching sing-box from: %s\n", singBoxExec)
	singBoxCmd := exec.Command(singBoxExec, "run", "-c", configPath)
	singBoxCmd.Stdout = os.Stdout
	singBoxCmd.Stderr = os.Stderr

	if err := singBoxCmd.Start(); err != nil {
		os.Remove(configPath)
		return fmt.Errorf("failed to start sing-box: %v", err)
	}

	// Wait for sing-box to start
	time.Sleep(1 * time.Second)

	// Check if process died immediately
	if singBoxCmd.ProcessState != nil && singBoxCmd.ProcessState.Exited() {
		os.Remove(configPath)
		return fmt.Errorf("sing-box started but died immediately")
	}

	// --- NETWORK GUARD START ---
	fmt.Println("DEBUG: Performing Network Check...")

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), nil, proxy.Direct)
	if err != nil {
		killSingBox(singBoxCmd)
		os.Remove(configPath)
		return fmt.Errorf("failed to create dialer: %v", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{Dial: dialer.Dial},
		Timeout:   30 * time.Second,
	}

	resp, err := httpClient.Get("http://clients3.google.com/generate_204")
	if err != nil {
		killSingBox(singBoxCmd)
		os.Remove(configPath)
		return fmt.Errorf("❌ TUNNEL DEAD: Connection failed: %v", err)
	}
	resp.Body.Close()
	fmt.Println("✅ Tunnel Connection: OK")

	// Проверка IP
	resp, err = httpClient.Get("http://ip-api.com/json")
	if err == nil {
		defer resp.Body.Close()
		var ipInfo struct {
			Query   string `json:"query"`
			Country string `json:"country"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ipInfo); err == nil {
			fmt.Printf("✅ Public IP: %s (%s)\n", ipInfo.Query, ipInfo.Country)
		}
	}
	// --- NETWORK GUARD END ---

	// --- TOKEN REFRESH CHECK ---
	// Check if token is expired or will expire within 5 minutes
	now := time.Now().Unix()
	expiresIn5Min := now + 5*60

	if profile.ExpiryTimestamp < expiresIn5Min {
		fmt.Println("🔄 Token expired or expiring soon, refreshing...")
		
		tokenResp, err := auth.RefreshAccessTokenViaProxy(profile.RefreshToken, localPort)
		if err != nil {
			fmt.Printf("⚠️ Token refresh failed: %v\n", err)
			fmt.Println("   Continuing with existing token...")
		} else {
			// Update profile with new token
			profile.AccessToken = tokenResp.AccessToken
			profile.ExpiryTimestamp = time.Now().Unix() + tokenResp.ExpiresIn
			
			// Save to store
			store.Profiles[profileName] = profile
			if err := store.Save(); err != nil {
				fmt.Printf("⚠️ Failed to save refreshed token: %v\n", err)
			} else {
				fmt.Println("✅ Token refreshed successfully!")
			}
		}
	}
	// --- TOKEN REFRESH END ---

	// 5. Инъекция
	fmt.Println("DEBUG: Cleaning up old processes...")
	utils.KillAntigravity()
	time.Sleep(1 * time.Second)

	dbPath := utils.GetAntigravityDBPath()
	fmt.Printf("DEBUG: Injecting identity into DB: %s\n", dbPath)
	if err := injection.InjectIdentity(dbPath, profile.AccessToken, profile.RefreshToken, profile.Email, profile.Name); err != nil {
		killSingBox(singBoxCmd)
		os.Remove(configPath)
		return fmt.Errorf("injection failed: %v", err)
	}

	// 6. Запуск IDE
	idePath := utils.GetAntigravityPath()
	fmt.Printf("DEBUG: Starting IDE from: %s\n", idePath)
	ideCmd, err := utils.StartAntigravity(idePath, localPort)
	if err != nil {
		killSingBox(singBoxCmd)
		os.Remove(configPath)
		return fmt.Errorf("failed to start IDE: %v", err)
	}

	fmt.Println("🚀 Antigravity launched!")
	config.SetActiveProfileName(profileName)

	// 7. Create session and run session menu
	activeSession = session.NewSession()
	activeSession.Start(profile, localPort, singBoxCmd, singBoxExec, configPath, ideCmd)

	// Setup Signal Handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	
	// Watch for signals in background
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal. Cleaning up...")
		if activeSession != nil && activeSession.IsActive() {
			fmt.Println("Stopping session...")
			activeSession.Stop() // Kills IDE and Tunnel
			time.Sleep(500 * time.Millisecond) // Wait for DB lock release
			session.ClearIDECredentials()
		}
		os.Exit(0)
	}()

	// Run session menu (blocks until user exits)
	runSessionMenu(activeSession, store)

	// Cleanup on exit (Manual Disconnect)
	fmt.Println("\nStopping tunnel...")
	activeSession.Stop() // Kill IDE and Tunnel first
	time.Sleep(500 * time.Millisecond)
	session.ClearIDECredentials() // Then clear credentials
	fmt.Println("Session ended.")
	return nil
}

var connectCmd = &cobra.Command{
	Use:   "connect [profile_name]",
	Short: "Connect to a proxy profile and launch Antigravity IDE",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		profileName := "default"
		if len(args) > 0 {
			profileName = args[0]
		}
		return RunConnect(profileName)
	},
}

func killSingBox(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func init() {
	rootCmd.AddCommand(connectCmd)
}