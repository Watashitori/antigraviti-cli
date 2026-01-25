package cmd

import (
	"antigravity-cli/internal/config"
	"antigravity-cli/internal/injection"
	"antigravity-cli/internal/tunnel"
	"antigravity-cli/internal/utils"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [profile_name]",
	Short: "Connect to a proxy profile and launch Antigravity IDE",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName := "default"
		if len(args) > 0 {
			profileName = args[0]
		}

		fmt.Printf("Connecting to profile: %s\n", profileName)

		// 1. Load profile from store
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}
		storePath := filepath.Join(homeDir, ".antigravity-cli", "profiles.json")
		store := config.NewStore(storePath)
		if err := store.Load(); err != nil {
			log.Fatalf("Failed to load profiles: %v", err)
		}

		profile, ok := store.GetProfile(profileName)
		if !ok {
			log.Fatalf("Profile '%s' not found. Use 'antigravity-cli profile add' to create one.", profileName)
		}

		// Check if profile has tokens (authorization required)
		if profile.AccessToken == "" || profile.AccessToken == "pending_auth" {
			log.Fatalf("Profile has no tokens. Please run 'antigravity-cli login %s' first.", profileName)
		}

		// 2. Find free port
		localPort := findFreePort(10000, 20000)
		fmt.Printf("Selected local port: %d\n", localPort)

		// 3. Generate tunnel config with proxy chaining
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
			log.Fatalf("Failed to generate tunnel config: %v", err)
		}
		// Defer removal of config file? Maybe keep it for debug or remove on exit.
		// For now, we leave it or better, clean it up on exit if possible.
		defer os.Remove(configPath)

		// 4. Start sing-box
		// We use command context to kill it when CLI exits? 
		// The prompt says "Start sing-box in a separate goroutine" and "Use select {} ... until Ctrl+C".
		// Actually, if we use exec.Command in a goroutine, we need to manage its lifecycle.
		// Better to start it and monitor it.
		
		singBoxCmd := exec.Command("sing-box", "run", "-c", configPath)
		singBoxCmd.Stdout = os.Stdout
		singBoxCmd.Stderr = os.Stderr

		go func() {
			fmt.Println("Starting sing-box...")
			if err := singBoxCmd.Run(); err != nil {
				// If manually killed, this error is expected.
				log.Printf("sing-box exited: %v", err)
				os.Exit(1) // Exit CLI if tunnel dies
			}
		}()

		// Wait a moment for sing-box to start
		time.Sleep(2 * time.Second) 
		fmt.Println("Tunnel started")

		// 5. Kill old process
		fmt.Println("Cleaning up old processes...")
		utils.KillAntigravity()
		time.Sleep(1 * time.Second)

		// 6. Inject Identity with decrypted tokens and profile info
		dbPath := utils.GetAntigravityDBPath()
		fmt.Printf("Injecting identity into %s...\n", dbPath)
		if err := injection.InjectIdentity(dbPath, profile.AccessToken, profile.RefreshToken, profile.Email, profile.Name); err != nil {
			log.Fatalf("Failed to inject identity: %v", err)
		}

		// 7. Start IDE
		idePath := utils.GetAntigravityPath()
		fmt.Printf("Starting Antigravity from %s...\n", idePath)
		if err := utils.StartAntigravity(idePath, localPort); err != nil {
			log.Fatalf("Failed to start Antigravity: %v", err)
		}

		fmt.Println("Antigravity launched via tunnel. Press Ctrl+C to stop.")

		// 8. Wait for signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		<-sigChan
		fmt.Println("\nStopping tunnel...")
		if singBoxCmd.Process != nil {
			singBoxCmd.Process.Kill()
		}
		fmt.Println("Exiting.")
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

func findFreePort(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 100; i++ {
		port := rand.Intn(max-min+1) + min
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			return port
		}
	}
	return min // Fallback
}
