package cmd

import (
	"antigravity-cli/internal/injection"
	"antigravity-cli/internal/utils"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var injectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Inject a cloud token into Antigravity IDE",
	RunE: func(cmd *cobra.Command, args []string) error {
		email, _ := cmd.Flags().GetString("email")
		name, _ := cmd.Flags().GetString("name")
		accessToken, _ := cmd.Flags().GetString("token")
		refreshToken, _ := cmd.Flags().GetString("refresh")
		kill, _ := cmd.Flags().GetBool("kill")

		if accessToken == "" {
			return fmt.Errorf("access token is required")
		}

		if kill {
			fmt.Println("Killing Antigravity process...")
			utils.KillAntigravity()
			time.Sleep(1 * time.Second)
		}

		dbPath := utils.GetAntigravityDBPath()
		fmt.Printf("Injecting token for %s into %s...\n", email, dbPath)
		
		err := injection.InjectIdentity(dbPath, accessToken, refreshToken, email, name)
		if err != nil {
			return fmt.Errorf("injection failed: %v", err)
		}

		fmt.Println("Successfully injected cloud token!")
		return nil
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