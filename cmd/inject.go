package cmd

import (
	"antigravity-cli/internal/injection"
	"antigravity-cli/internal/utils"
	"fmt"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var injectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Inject a cloud token into Antigravity IDE",
	Run: func(cmd *cobra.Command, args []string) {
		email, _ := cmd.Flags().GetString("email")
		name, _ := cmd.Flags().GetString("name")
		accessToken, _ := cmd.Flags().GetString("token")
		refreshToken, _ := cmd.Flags().GetString("refresh")
		kill, _ := cmd.Flags().GetBool("kill")

		if accessToken == "" {
			log.Fatal("Access token is required")
		}

		if kill {
			// Try to kill process first
			// IsProcessRunning didn't exist in utils/process.go view earlier?
			// Let's check process.go content again or just call KillAntigravity directly which calls taskkill/pkill.
			// The previous inject.go used utils.IsProcessRunning() and utils.KillProcess().
			// But utils/process.go only has KillAntigravity and StartAntigravity?
			// Let's check process.go again.
			// If they don't exist, I should use KillAntigravity.
			fmt.Println("Killing Antigravity process...")
			utils.KillAntigravity()
			time.Sleep(1 * time.Second)
		}

		dbPath := utils.GetAntigravityDBPath()
		fmt.Printf("Injecting token for %s into %s...\n", email, dbPath)
		
		err := injection.InjectIdentity(dbPath, accessToken, refreshToken, email, name)
		if err != nil {
			log.Fatalf("Injection failed: %v", err)
		}

		fmt.Println("Successfully injected cloud token!")
	},
}

func init() {
	rootCmd.AddCommand(injectCmd)
	injectCmd.Flags().StringP("email", "e", "", "Email address (informational)")
	injectCmd.Flags().StringP("name", "n", "", "Display name")
	injectCmd.Flags().StringP("token", "t", "", "Access Token (required)")
	injectCmd.Flags().StringP("refresh", "r", "", "Refresh Token")
	injectCmd.Flags().Int64("expiry", 0, "Expiry timestamp (seconds) - Ignored in this version")
	injectCmd.Flags().BoolP("kill", "k", false, "Kill Antigravity process before injection")
}
