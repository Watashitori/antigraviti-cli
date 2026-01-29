package cmd

import (
	"antigravity-cli/internal/config"
	"antigravity-cli/internal/session"
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"text/template"
	"time"

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

var debugLogger *log.Logger

func init() {
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, "antigravity_debug.log")
	
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("FAILED TO CREATE LOG: %v\n", err)
		return
	}
	debugLogger = log.New(f, "TUI: ", log.LstdFlags)
	debugLogger.Printf("---- Session Started ----")
	fmt.Printf("Debug log: %s\n", logPath)
}

// waitUser делает надежную паузу
func waitUser() {
	fmt.Println("\nPress 'Enter' to continue...")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}

func runInteractiveMenu() {
	defer func() {
		if r := recover(); r != nil {
			if debugLogger != nil {
				debugLogger.Printf("PANIC: %v\nStack: %s", r, string(debug.Stack()))
			}
			fmt.Printf("CRASH: %v\n", r)
			// Wait unconditionally on panic
			time.Sleep(20 * time.Second)
		}
	}()

	for {
		// clearScreen() REMOVED FOR DEBUGGING
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
				if debugLogger != nil {
					debugLogger.Println("Exiting due to interrupt/EOF")
				}
				os.Exit(0)
			}
			if debugLogger != nil {
				debugLogger.Printf("Prompt error: %v", err)
			}
			fmt.Printf("Prompt error: %v\n", err)
			waitUser()
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

// menuItem - обертка для красивого отображения в меню
type menuItem struct {
	Label   string
	Profile config.Profile
	IsBack  bool
}

func handleSelectAccount() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Error getting home dir: %v", err)
		waitUser()
		return
	}
	store := config.NewStore(filepath.Join(homeDir, ".antigravity-cli", "profiles.json"))
	if err := store.Load(); err != nil {
		log.Printf("Error loading profiles: %v", err)
		waitUser()
		return
	}

	var profiles []config.Profile
	for _, p := range store.Profiles {
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	var items []menuItem
	for _, p := range profiles {
		label := fmt.Sprintf("%s (%s)", p.Name, p.Email)
		items = append(items, menuItem{Label: label, Profile: p, IsBack: false})
	}
	items = append(items, menuItem{Label: "Back", IsBack: true})

	if len(items) == 1 { 
		fmt.Println("No profiles found.")
		waitUser()
		return
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ .Label }}",
		Active:   "-> {{ .Label | cyan }}",
		Inactive: "   {{ .Label }}",
		Selected: "-> {{ .Label | cyan }}",
		Details: `
--------- Profile Details ---------
{{ if .IsBack }}
Go back to main menu
{{ else }}
{{ "Name:" | faint }}	{{ .Profile.Name }}
{{ "Email:" | faint }}	{{ .Profile.Email }}
{{ "Proxy:" | faint }}	{{ .Profile.ProxyHost }}:{{ .Profile.ProxyPort }}
{{ "Tunnel:" | faint }}	{{ .Profile.UseSystemTunnel }}
{{ end }}
-----------------------------------`,
		FuncMap: template.FuncMap{
			"cyan":  promptui.FuncMap["cyan"],
			"faint": promptui.FuncMap["faint"],
		},
	}

	prompt := promptui.Select{
		Label:     "Select Account",
		Items:     items,
		Templates: templates,
		Size:      10,
		Stdout:    &BellSkipper{},
	}

	i, _, err := prompt.Run()
	if err != nil {
		return
	}

	selectedItem := items[i]
	if selectedItem.IsBack {
		return
	}

	handleProfileActions(selectedItem.Profile, store)
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
		// clearScreen() REMOVED
		fmt.Printf("--- Launching Profile: %s ---\n", profile.Name)
		if debugLogger != nil {
			debugLogger.Printf("Calling RunConnect for %s", profile.Name)
		}
		err := RunConnect(profile.Name)
		if err != nil {
			if debugLogger != nil {
				debugLogger.Printf("RunConnect error: %v", err)
			}
			fmt.Printf("\n❌ ERROR: %v\n", err)
		}
		waitUser()

	case "Login":
		if loginCmd != nil {
			err := loginCmd.RunE(loginCmd, []string{profile.Name})
			if err != nil {
				fmt.Printf("\n❌ Error: %v\n", err)
			}
		} else {
			fmt.Println("Login command not available.")
		}
		waitUser()

	case "Delete":
		confirmPrompt := promptui.Prompt{
			Label:     fmt.Sprintf("Are you sure you want to delete profile '%s'", profile.Name),
			IsConfirm: true,
		}
		_, err := confirmPrompt.Run()
		if err == nil {
			delete(store.Profiles, profile.Name)
			if err := store.Save(); err != nil {
				fmt.Printf("Error deleting profile: %v\n", err)
			} else {
				fmt.Println("Profile deleted.")
			}
		}
		waitUser()

	case "Back":
		return
	}
}

func handleAddAccount() {
	// Wizard
	proxyTypePrompt := promptui.Select{
		Label: "Proxy Type",
		Items: []string{"socks5", "http"},
		Stdout: &BellSkipper{},
	}
	_, proxyType, err := proxyTypePrompt.Run()
	if err != nil { return }

	proxyHostPrompt := promptui.Prompt{
		Label: "Proxy Host",
		Validate: func(input string) error {
			if len(input) == 0 { return errors.New("host cannot be empty") }
			return nil
		},
	}
	proxyHost, err := proxyHostPrompt.Run()
	if err != nil { return }

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

	proxyUserPrompt := promptui.Prompt{
		Label: "Proxy User",
	}
	proxyUser, err := proxyUserPrompt.Run()
	if err != nil { return }

	proxyPassPrompt := promptui.Prompt{
		Label: "Proxy Password",
		Mask: '*',
	}
	proxyPass, err := proxyPassPrompt.Run()
	if err != nil { return }

	sysTunnelPrompt := promptui.Select{
		Label: "Use System Tunnel?",
		Items: []string{"Yes", "No"},
		Stdout: &BellSkipper{},
	}
	_, sysTunnelRes, err := sysTunnelPrompt.Run()
	if err != nil { return }
	isSystemTunnel := (sysTunnelRes == "Yes")

	namePrompt := promptui.Prompt{
		Label: "Profile Name",
		Validate: func(input string) error {
			if len(input) == 0 { return errors.New("name cannot be empty") }
			return nil
		},
	}
	name, err := namePrompt.Run()
	if err != nil { return }

	emailPrompt := promptui.Prompt{
		Label: "Google Email",
	}
	email, err := emailPrompt.Run()
	if err != nil { return }

	newProfile := config.Profile{
		Name:            name,
		Email:           email,
		ProxyScheme:     proxyType,
		ProxyHost:       proxyHost,
		ProxyPort:       proxyPort,
		ProxyUser:       proxyUser,
		ProxyPass:       proxyPass,
		UseSystemTunnel: isSystemTunnel,
		AccessToken:     "pending_auth",
		RefreshToken:    "pending_auth",
		ExpiryTimestamp: 0,
	}

	homeDir, _ := os.UserHomeDir()
	store := config.NewStore(filepath.Join(homeDir, ".antigravity-cli", "profiles.json"))
	store.Load()
	
	if err := store.AddProfile(newProfile); err != nil {
		fmt.Printf("Failed to save profile: %v\n", err)
	} else {
		fmt.Println("Profile saved successfully!")
	}
	
	waitUser()
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

type BellSkipper struct{}

func (bs *BellSkipper) Write(b []byte) (int, error) {
	const bell = 7
	if len(b) == 1 && b[0] == bell {
		return 0, nil
	}
	var filtered []byte
	for _, byteVal := range b {
		if byteVal != bell {
			filtered = append(filtered, byteVal)
		}
	}
	_, err := os.Stdout.Write(filtered)
	return len(b), err
}

func (bs *BellSkipper) Close() error {
	return nil
}

// runSessionMenu displays the session control menu during an active connection
func runSessionMenu(sess *session.Session, store *config.Store) {
	for {
		fmt.Printf("\n🔒 Connected: %s\n", sess.ProfileName())

		prompt := promptui.Select{
			Label: "Session Menu",
			Items: []string{
				"🔄 Switch Account",
				"📊 Check Status",
				"🚪 Disconnect",
			},
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
				return // Exit on Ctrl+C
			}
			fmt.Printf("Prompt error: %v\n", err)
			continue
		}

		switch result {
		case "🔄 Switch Account":
			newProfile := selectProfileForSwitch(store, sess.ProfileName())
			if newProfile != nil {
				if err := sess.SwitchProfile(*newProfile); err != nil {
					fmt.Printf("❌ Switch failed: %v\n", err)
					waitUser()
				}
			}

		case "📊 Check Status":
			sess.ShowStatus()
			waitUser()

		case "🚪 Disconnect":
			return
		}
	}
}

// selectProfileForSwitch shows a profile picker for switching
func selectProfileForSwitch(store *config.Store, currentProfile string) *config.Profile {
	// Reload profiles
	if err := store.Load(); err != nil {
		fmt.Printf("Error loading profiles: %v\n", err)
		return nil
	}

	var profiles []config.Profile
	for _, p := range store.Profiles {
		if p.Name != currentProfile { // Exclude current profile
			profiles = append(profiles, p)
		}
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	if len(profiles) == 0 {
		fmt.Println("No other profiles available.")
		waitUser()
		return nil
	}

	var items []menuItem
	for _, p := range profiles {
		label := fmt.Sprintf("%s (%s)", p.Name, p.Email)
		items = append(items, menuItem{Label: label, Profile: p, IsBack: false})
	}
	items = append(items, menuItem{Label: "Cancel", IsBack: true})

	templates := &promptui.SelectTemplates{
		Label:    "{{ .Label }}",
		Active:   "-> {{ .Label | cyan }}",
		Inactive: "   {{ .Label }}",
		Selected: "-> {{ .Label | cyan }}",
		Details: `
--------- Profile Details ---------
{{ if .IsBack }}
Cancel and go back
{{ else }}
{{ "Name:" | faint }}	{{ .Profile.Name }}
{{ "Email:" | faint }}	{{ .Profile.Email }}
{{ "Proxy:" | faint }}	{{ .Profile.ProxyHost }}:{{ .Profile.ProxyPort }}
{{ end }}
-----------------------------------`,
		FuncMap: template.FuncMap{
			"cyan":  promptui.FuncMap["cyan"],
			"faint": promptui.FuncMap["faint"],
		},
	}

	prompt := promptui.Select{
		Label:     "Select Profile to Switch",
		Items:     items,
		Templates: templates,
		Size:      10,
		Stdout:    &BellSkipper{},
	}

	i, _, err := prompt.Run()
	if err != nil {
		return nil
	}

	selectedItem := items[i]
	if selectedItem.IsBack {
		return nil
	}

	return &selectedItem.Profile
}