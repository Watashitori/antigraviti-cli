package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"antigravity-cli/internal/auth"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [profile_name]",
	Short: "Authenticate a profile with Google OAuth",
	Long: `Authenticate a profile with Google OAuth.
This will open your browser for Google login, then capture the tokens
and save them encrypted to the profile.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogin(cmd, args)
	},
}

func runLogin(cmd *cobra.Command, args []string) error {
	profileName := args[0]

	// Load the store and get the profile
	store, err := getStore()
	if err != nil {
		return fmt.Errorf("error loading store: %v", err)
	}

	profile, exists := store.GetProfile(profileName)
	if !exists {
		return fmt.Errorf("profile '%s' not found. Use 'antigravity profile add' to create one first", profileName)
	}

	// Channel to receive the authorization code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Create HTTP server for OAuth callback
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    ":8888",
		Handler: mux,
	}

	mux.HandleFunc("/oauth-callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no authorization code received"
			}
			errChan <- fmt.Errorf("OAuth error: %s", errMsg)
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><body><h1>Authentication Failed</h1><p>%s</p><p>You can close this window.</p></body></html>", errMsg)
			return
		}

		codeChan <- code
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><body><h1>Authentication Successful!</h1><p>You can close this window and return to the CLI.</p></body></html>")
	})

	// Start the server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("failed to start callback server: %w", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Generate and open the auth URL
	authURL := auth.GetAuthURL()
	fmt.Println("Opening browser for Google authentication...")
	fmt.Printf("If the browser doesn't open automatically, visit:\n%s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Warning: Could not open browser automatically: %v\n", err)
	}

	fmt.Println("Waiting for authentication...")

	// Wait for the code or error
	var authCode string
	select {
	case authCode = <-codeChan:
		// Success, continue
	case err := <-errChan:
		shutdownServer(server)
		return fmt.Errorf("error: %v", err)
	case <-time.After(5 * time.Minute):
		shutdownServer(server)
		return fmt.Errorf("error: authentication timed out after 5 minutes")
	}

	// Shutdown the server
	shutdownServer(server)

	// Exchange the code for tokens
	fmt.Println("Exchanging authorization code for tokens...")
	tokenResp, err := auth.ExchangeCode(authCode)
	if err != nil {
		return fmt.Errorf("error exchanging code: %v", err)
	}

	// Get user info to verify email
	userInfo, err := auth.GetUserInfo(tokenResp.AccessToken)
	if err != nil {
		fmt.Printf("Warning: Could not verify user email: %v\n", err)
	} else {
		if !strings.EqualFold(userInfo.Email, profile.Email) {
			fmt.Printf("Warning: Authenticated email (%s) does not match profile email (%s)\n", userInfo.Email, profile.Email)
		} else {
			fmt.Printf("Email verified: %s\n", userInfo.Email)
		}
	}

	// Calculate token expiry timestamp
	expiryTimestamp := time.Now().Unix() + tokenResp.ExpiresIn

	// Update the profile with tokens
	profile.AccessToken = tokenResp.AccessToken
	profile.RefreshToken = tokenResp.RefreshToken
	profile.ExpiryTimestamp = expiryTimestamp

	// Save the updated profile
	store.Profiles[profileName] = profile
	if err := store.Save(); err != nil {
		return fmt.Errorf("error saving tokens: %v", err)
	}

	fmt.Println("\n✓ Login successful! Tokens have been saved and encrypted.")
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}

func shutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func init() {
	// No additional flags needed for login command
}
