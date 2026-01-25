package cmd

import (
	"antigravity-cli/internal/config"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"

	"github.com/manifoldco/promptui"
)

const asciiHeader = `                _   _                       _ _                 _____ _      _ 
    /\         | | (_)                     (_) |               / ____| |    (_)
   /  \   _ __ | |_ _  __ _ _ __ __ ___   ___| |_ _   _ ______| |    | |     _ 
  / /\ \ | '_ \| __| |/ _` + "`" + ` | '__/ _` + "`" + ` \ \ / / | __| | | |______| |    | |    | |
 / ____ \| | | | |_| | (_| | | | (_| |\ V /| | |_| |_| |      | |____| |____| |
/_/    \_\_| |_|\__|_|\__, |_|  \__,_| \_/ |_|\__|\__, |       \_____|______|_|
                       __/ |                       __/ |                       
                      |___/                       |___/                        
`

func runInteractiveMenu() {
	for {
		clearScreen()
		fmt.Print(asciiHeader)
		active := config.GetActiveProfileName()
		if active == "" {
			active = "None"
		}
		fmt.Printf("Current Account: %s\n\n", active)

		prompt := promptui.Select{
			Label: "Main Menu",
			Items: []string{"Select Account", "Add New Account", "Exit"},
			Templates: &promptui.SelectTemplates{
				Active:   "-> {{ . | cyan }}",
				Inactive: "   {{ . }}",
				Selected: "-> {{ . | cyan }}",
			},
			HideSelected: true,
			Stdout:       &BellSkipper{},
		}

		_, result, err := prompt.Run()
		if err != nil {
			if err == promptui.ErrInterrupt {
				os.Exit(0)
			}
			return
		}

		switch result {
		case "Select Account":
			handleSelectAccount()
		case "Add New Account":
			handleAddAccount()
		case "Exit":
			os.Exit(0)
		}
	}
}

func handleSelectAccount() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Error getting home dir: %v", err)
		return
	}
	store := config.NewStore(filepath.Join(homeDir, ".antigravity-cli", "profiles.json"))
	if err := store.Load(); err != nil {
		log.Printf("Error loading profiles: %v", err)
		return
	}

	var items []interface{}
	for _, p := range store.Profiles {
		items = append(items, p)
	}
	// Sort by name for consistency
	sort.Slice(items, func(i, j int) bool {
		return items[i].(config.Profile).Name < items[j].(config.Profile).Name
	})
	// Add Back option
	items = append(items, "Back")

	if len(items) == 1 { // Only "Back" option
		fmt.Println("No profiles found.")
		prompt := promptui.Prompt{
			Label: "Press Enter to go back",
		}
		prompt.Run()
		return
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "-> {{ if eq . \"Back\" }}{{ . | cyan }}{{ else }}{{ .Name | cyan }} ({{ .ProxyHost }}){{ end }}",
		Inactive: "   {{ if eq . \"Back\" }}{{ . }}{{ else }}{{ .Name }} ({{ .ProxyHost }}){{ end }}",
		Selected: "-> {{ if eq . \"Back\" }}{{ . | cyan }}{{ else }}{{ .Name | cyan }}{{ end }}",
		Details: `
--------- Profile Details ---------
{{ if eq . "Back" }}
Go back to main menu
{{ else }}
{{ "Name:" | faint }}	{{ .Name }}
{{ "Email:" | faint }}	{{ .Email }}
{{ "Proxy:" | faint }}	{{ .ProxyHost }}:{{ .ProxyPort }}
{{ "Tunnel:" | faint }}	{{ .UseSystemTunnel }}
{{ end }}
-----------------------------------`,
	}

	prompt := promptui.Select{
		Label:     "Select Account",
		Items:     items,
		Templates: templates,
		Size:      10,
		Stdout:    &BellSkipper{},
	}

	i, result, err := prompt.Run()
	if err != nil {
		return
	}

	if result == "Back" {
		return
	}

	selectedProfile := items[i].(config.Profile)
	handleProfileActions(selectedProfile, store)
}

func handleProfileActions(profile config.Profile, store *config.Store) {
	prompt := promptui.Select{
		Label: "Action",
		Items: []string{"Connect", "Login", "Delete", "Back"},
		Stdout: &BellSkipper{},
	}

	_, result, err := prompt.Run()
	if err != nil {
		return
	}

	switch result {
	case "Connect":
		// Invoke connect command logic
		connectCmd.SetArgs([]string{profile.Name})
		err := connectCmd.Execute()
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		}
		// After connect returns (e.g. user stopped tunnel), we return to menu
		fmt.Println("Press Enter to return to menu...")
		fmt.Scanln()

	case "Login":
		// Invoke login command logic
		if loginCmd != nil {
			loginCmd.SetArgs([]string{profile.Name})
			err := loginCmd.Execute()
			if err != nil {
				fmt.Printf("\nError: %v\n", err)
			}
		} else {
			fmt.Println("Login command not available.")
		}
		fmt.Println("Press Enter to continue...")
		fmt.Scanln()

	case "Delete":
		confirmPrompt := promptui.Prompt{
			Label:     fmt.Sprintf("Are you sure you want to delete profile '%s'", profile.Name),
			IsConfirm: true,
		}
		_, err := confirmPrompt.Run()
		if err == nil {
			// User confirmed
			delete(store.Profiles, profile.Name)
			// We need to save manually since store.RemoveProfile might not exist or be accessible
			// store.Save() handles encryption
			if err := store.Save(); err != nil {
				fmt.Printf("Error deleting profile: %v\n", err)
			} else {
				fmt.Println("Profile deleted.")
			}
		}

	case "Back":
		return
	}
}

func handleAddAccount() {
	// Wizard
	// 1. Proxy Type
	proxyTypePrompt := promptui.Select{
		Label: "Proxy Type",
		Items: []string{"socks5", "http"},
		Stdout: &BellSkipper{},
	}
	_, proxyType, err := proxyTypePrompt.Run()
	if err != nil { return } // Handles Ctrl+C (ErrInterrupt)

	// 2. Proxy Host
	proxyHostPrompt := promptui.Prompt{
		Label: "Proxy Host",
		Validate: func(input string) error {
			if len(input) == 0 { return errors.New("host cannot be empty") }
			return nil
		},
	}
	proxyHost, err := proxyHostPrompt.Run()
	if err != nil { return }

	// 3. Proxy Port
	proxyPortPrompt := promptui.Prompt{
		Label: "Proxy Port",
		Validate: func(input string) error {
			_, err := strconv.Atoi(input)
			if err != nil { return errors.New("invalid port number") }
			return nil
		},
	}
	proxyPortStr, err := proxyPortPrompt.Run()
	if err != nil { return }
	proxyPort, _ := strconv.Atoi(proxyPortStr)

	// 4. Proxy User
	proxyUserPrompt := promptui.Prompt{
		Label: "Proxy User",
	}
	proxyUser, err := proxyUserPrompt.Run()
	if err != nil { return }

	// 5. Proxy Password
	proxyPassPrompt := promptui.Prompt{
		Label: "Proxy Password",
		Mask: '*',
	}
	proxyPass, err := proxyPassPrompt.Run()
	if err != nil { return }

	// 6. Use System Tunnel?
	sysTunnelPrompt := promptui.Select{
		Label: "Use System Tunnel?",
		Items: []string{"Yes", "No"},
		Stdout: &BellSkipper{},
	}
	_, sysTunnelRes, err := sysTunnelPrompt.Run()
	if err != nil { return }
	useSysTunnel := (sysTunnelRes == "Yes")

	// 7. Profile Name
	namePrompt := promptui.Prompt{
		Label: "Profile Name",
		Validate: func(input string) error {
			if len(input) == 0 { return errors.New("name cannot be empty") }
			return nil
		},
	}
	name, err := namePrompt.Run()
	if err != nil { return }

	// 8. Google Email
	emailPrompt := promptui.Prompt{
		Label: "Google Email",
	}
	email, err := emailPrompt.Run()
	if err != nil { return }

	// Save
	newProfile := config.Profile{
		Name:            name,
		Email:           email,
		ProxyScheme:     proxyType,
		ProxyHost:       proxyHost,
		ProxyPort:       proxyPort,
		ProxyUser:       proxyUser,
		ProxyPass:       proxyPass,
		UseSystemTunnel: useSysTunnel,
	}

	homeDir, _ := os.UserHomeDir()
	store := config.NewStore(filepath.Join(homeDir, ".antigravity-cli", "profiles.json"))
	// Load first to not overwrite others
	store.Load()
	
	if err := store.AddProfile(newProfile); err != nil {
		fmt.Printf("Failed to save profile: %v\n", err)
	} else {
		fmt.Println("Profile saved successfully!")
	}
	
	// Wait a bit
	fmt.Println("Press Enter to continue...")
	fmt.Scanln()
}

func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// BellSkipper implements an io.WriteCloser that skips the bell character (\a).
// This prevents annoying sounds on Windows terminals during navigation.
type BellSkipper struct{}

func (bs *BellSkipper) Write(b []byte) (int, error) {
	const bell = 7 // ASCII \a
	if len(b) == 1 && b[0] == bell {
		return 0, nil
	}
	// For larger chunks, filter out bells
	// This is a bit expensive for large writes but efficient enough for TUI
	var filtered []byte
	for _, byteVal := range b {
		if byteVal != bell {
			filtered = append(filtered, byteVal)
		}
	}
	// Note: We return len(b) to pretend we wrote everything, to satisfy io.Writer contract
	_, err := os.Stdout.Write(filtered)
	return len(b), err
}

func (bs *BellSkipper) Close() error {
	return nil
}

// Helper to get a configured Select with suppressed bell
func newSelect(label string, items interface{}, templates *promptui.SelectTemplates) promptui.Select {
	return promptui.Select{
		Label:     label,
		Items:     items,
		Templates: templates,
		Stdout:    &BellSkipper{},
	}
}
