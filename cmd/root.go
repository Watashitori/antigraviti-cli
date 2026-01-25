package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "antigravity",
	Short: "Antigravity CLI Manager",
	Long:  `A CLI port of the AntigravityManager logic for managing cloud accounts and injection.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			runInteractiveMenu()
		}
	},
}

func Execute() {
	// Disable the "This is a command line tool" check to allow TUI on double-click
	cobra.MousetrapHelpText = ""
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
