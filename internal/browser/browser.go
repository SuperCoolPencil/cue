package browser

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenURL opens the specified URL in the user's default web browser cross-platform.
func OpenURL(rawURL string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if isWSL() {
			cmd = exec.Command("wslview", rawURL)
			if err := cmd.Start(); err == nil {
				return nil
			}
			cmd = exec.Command("powershell.exe", "-NoProfile", "-Command", "Start-Process", `"`+rawURL+`"`)
		} else {
			cmd = exec.Command("xdg-open", rawURL)
		}
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}

	return cmd.Start()
}

func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft")
}
