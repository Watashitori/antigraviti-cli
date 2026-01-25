package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "antigravity",
	Short: "Antigravity CLI Manager",
	Long:  `A CLI port of the AntigravityManager logic for managing cloud accounts and injection.`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags can be defined here
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(loginCmd)
}
