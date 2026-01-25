package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// KillAntigravity attempts to terminate running Antigravity/Code processes.
// It ignores errors if the process is not found.
func KillAntigravity() {
	switch runtime.GOOS {
	case "windows":
		// Taskkill for both Antigravity.exe and Code.exe
		exec.Command("taskkill", "/F", "/IM", "Antigravity.exe").Run()
		exec.Command("taskkill", "/F", "/IM", "Code.exe").Run()
		return
	default:
		// pkill for Unix-like systems
		exec.Command("pkill", "Antigravity").Run()
		exec.Command("pkill", "Code").Run()
		return
	}
	// We deliberately ignore errors (e.g., if process isn't running)
}

// StartAntigravity launches the IDE with the specified proxy environment variables.
// It starts the process in a detached mode so it survives CLI exit.
func StartAntigravity(idePath string, localPort int) error {
	cmd := exec.Command(idePath)

	// Set environment variables for the NEW process, inheriting the current environment
	cmd.Env = os.Environ()
	proxyUrl := fmt.Sprintf("socks5://127.0.0.1:%d", localPort)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("ALL_PROXY=%s", proxyUrl),
		fmt.Sprintf("HTTP_PROXY=%s", proxyUrl),
		fmt.Sprintf("HTTPS_PROXY=%s", proxyUrl),
	)

	// Detach the process
	return startDetached(cmd)
}

// startDetached is a platform-specific helper to run a command detached
func startDetached(cmd *exec.Cmd) error {
	// On Windows, Go's exec.Start() usually doesn't wait, but to be truly "detached" 
	// from the console window or parent, we might need specific flags.
	// For cross-platform simplicity in this context (and typical Go behavior),
	// just Start() should be enough if we don't call Wait().
	// However, usually we might want to set SysProcAttr.

	// Since we are not strictly required to use complex SysProcAttr unless standard Start fails to detach visually,
	// checking standard behavior:
	// Go's exec.Command().Start() starts the process. If we exit the main program, 
	// the child process might stay alive unless it's in the same process group and receives a signal.
	
	// For robust detachment on Windows, usually nothing special is needed if we simply don't Wait().
	// But let's verify if we need `creationflags`.
	
	// If specific OS behavior is needed (like hiding window), we can add it here.
	// For now, standard Start() is used.
	
	return cmd.Start()
}
