package cmd

import (
	"antigravity-cli/internal/injection"
	"fmt"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync account from Antigravity IDE",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Syncing from IDE...")
		account, err := injection.SyncFromIDE()
		if err != nil {
			return fmt.Errorf("sync failed: %v", err)
		}

		fmt.Printf("found account: %s (%s)\n", account.Email, account.Name)
		fmt.Printf("token: %s...\n", account.AccessToken[:10])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}