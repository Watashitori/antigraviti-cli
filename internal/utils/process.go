package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// KillAntigravity attempts to terminate running Antigravity/Code processes.
func KillAntigravity() {
	switch runtime.GOOS {
	case "windows":
		exec.Command("taskkill", "/F", "/IM", "Antigravity.exe").Run()
		exec.Command("taskkill", "/F", "/IM", "Code.exe").Run()
		return
	default:
		exec.Command("pkill", "Antigravity").Run()
		exec.Command("pkill", "Code").Run()
		return
	}
}

// StartAntigravity launches the IDE with the specified proxy environment variables.
func StartAntigravity(idePath string, localPort int) (*exec.Cmd, error) {
	cmd := exec.Command(idePath)

	// Set environment variables for the NEW process
	cmd.Env = os.Environ()
	proxyUrl := fmt.Sprintf("socks5://127.0.0.1:%d", localPort)
	
	// ВАЖНО: Добавляем NO_PROXY, чтобы локальный трафик не шел в туннель
	noProxy := "localhost,127.0.0.1,::1,0.0.0.0"

	cmd.Env = append(cmd.Env,
		fmt.Sprintf("ALL_PROXY=%s", proxyUrl),
		fmt.Sprintf("HTTP_PROXY=%s", proxyUrl),
		fmt.Sprintf("HTTPS_PROXY=%s", proxyUrl),
		fmt.Sprintf("NO_PROXY=%s", noProxy),
	)

	return startDetached(cmd)
}

func startDetached(cmd *exec.Cmd) (*exec.Cmd, error) {
	err := cmd.Start()
	return cmd, err
}