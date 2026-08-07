// Package browser opens a URL in the user's default browser. Failures are
// non-fatal: the URL is already printed to stderr (spec §12).
package browser

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Open launches url in a browser. If MDV_BROWSER is set it takes precedence.
// Any error is returned but callers are expected to continue regardless.
func Open(url string) error {
	if custom := os.Getenv("MDV_BROWSER"); custom != "" {
		fields := strings.Fields(custom)
		args := append(fields[1:], url)
		return exec.Command(fields[0], args...).Start()
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
