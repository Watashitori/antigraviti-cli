package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"antigravity-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	name            string
	email           string
	proxyHost       string
	proxyPort       int
	proxyUser       string
	proxyPass       string
	proxyType       string
	useSystemTunnel bool
)

// profileCmd represents the profile command
var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage profiles",
	Long:  `Add, list, and remove profiles for Antigravity.`,
}

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new profile",
	Run: func(cmd *cobra.Command, args []string) {
		if name == "" {
			fmt.Println("Error: --name is required")
			return
		}
		if email == "" {
			fmt.Println("Error: --email is required")
			return
		}

		store, err := getStore()
		if err != nil {
			fmt.Printf("Error loading store: %v\n", err)
			return
		}

		profile := config.Profile{
			Name:            name,
			Email:           email,
			ProxyScheme:     proxyType,
			ProxyHost:       proxyHost,
			ProxyPort:       proxyPort,
			ProxyUser:       proxyUser,
			ProxyPass:       proxyPass,
			UseSystemTunnel: useSystemTunnel,
			// Initialize sensitive data with placeholders to ensure encryption works
			AccessToken:     "pending_auth",
			RefreshToken:    "pending_auth",
			ExpiryTimestamp: 0,
		}

		if err := store.AddProfile(profile); err != nil {
			fmt.Printf("Error adding profile: %v\n", err)
			return
		}

		fmt.Printf("Profile '%s' added successfully.\n", name)
	},
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	Run: func(cmd *cobra.Command, args []string) {
		store, err := getStore()
		if err != nil {
			fmt.Printf("Error loading store: %v\n", err)
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Name\tEmail\tProxy\tTunnel Mode")
		
		for _, p := range store.Profiles {
			proxy := "None"
			if p.ProxyHost != "" {
				proxy = fmt.Sprintf("%s://%s:%d", p.ProxyScheme, p.ProxyHost, p.ProxyPort)
			}
			tunnelMode := "Direct"
			if p.UseSystemTunnel {
				tunnelMode = "System Tunnel (VLESS)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Email, proxy, tunnelMode)
		}
		w.Flush()
	},
}

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a profile",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName := args[0]
		store, err := getStore()
		if err != nil {
			fmt.Printf("Error loading store: %v\n", err)
			return
		}

		if _, exists := store.Profiles[profileName]; !exists {
			fmt.Printf("Profile '%s' not found.\n", profileName)
			return
		}

		delete(store.Profiles, profileName)
		
		// We need to save manually since Store doesn't have a specific RemoveProfile method exposed with saving, 
		// but we can call Save().
		if err := store.Save(); err != nil {
			fmt.Printf("Error saving store after removal: %v\n", err)
			return
		}

		fmt.Printf("Profile '%s' removed successfully.\n", profileName)
	},
}

func init() {
	// add flags
	addCmd.Flags().StringVar(&name, "name", "", "Profile name")
	addCmd.Flags().StringVar(&email, "email", "", "Email address")
	addCmd.Flags().StringVar(&proxyHost, "proxy-host", "", "Proxy host")
	addCmd.Flags().IntVar(&proxyPort, "proxy-port", 0, "Proxy port")
	addCmd.Flags().StringVar(&proxyUser, "proxy-user", "", "Proxy username")
	addCmd.Flags().StringVar(&proxyPass, "proxy-pass", "", "Proxy password")
	addCmd.Flags().StringVar(&proxyType, "proxy-type", "socks5", "Proxy type (socks5/http)")
	addCmd.Flags().BoolVar(&useSystemTunnel, "use-system-tunnel", false, "Use system tunnel (VLESS)")

	profileCmd.AddCommand(addCmd)
	profileCmd.AddCommand(listCmd)
	profileCmd.AddCommand(removeCmd)
}

func getStore() (*config.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	// Store in ~/.antigravity-cli/profiles.json
	configDir := filepath.Join(home, ".antigravity-cli")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	
	storePath := filepath.Join(configDir, "profiles.json")
	store := config.NewStore(storePath)
	
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}
