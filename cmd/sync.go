package cmd

import (
	"antigravity-cli/internal/injection"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync account from Antigravity IDE",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Syncing from IDE...")
		account, err := injection.SyncFromIDE()
		if err != nil {
			log.Fatalf("Sync failed: %v", err)
		}

		fmt.Printf("found account: %s (%s)\n", account.Email, account.Name)
		fmt.Printf("token: %s...\n", account.AccessToken[:10])
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
