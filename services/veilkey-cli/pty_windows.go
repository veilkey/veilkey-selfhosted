//go:build windows

package main

import (
	"fmt"
	"os"
)

func cmdWrapPty(args []string, apiURL, logPath, patternsFile string) {
	fmt.Fprintln(os.Stderr, "ERROR: wrap-pty is not supported on Windows (no PTY support).")
	fmt.Fprintln(os.Stderr, "Use 'veilkey-cli wrap <command>' instead.")
	os.Exit(1)
}
