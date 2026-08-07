//go:build windows

package main

import (
	"fmt"
	"io"
	"net"
)

// Background/daemon mode relies on POSIX session detachment and listener-fd
// inheritance, which are not implemented for Windows yet. Foreground mode works
// on all platforms.

func inheritedListener() (net.Listener, bool, error) { return nil, false, nil }

func registerDaemonInstance(root, host string, port int, url string) func() {
	return func() {}
}

func spawnDaemon(opts options, root, openPath string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "mdv: --daemon is not supported on Windows yet; run in the foreground instead")
	return 1
}

func cmdStop(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "mdv: `mdv stop` is not supported on Windows yet")
	return 1
}

func cmdList(stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "mdv: `mdv ls` is not supported on Windows yet")
	return 1
}
