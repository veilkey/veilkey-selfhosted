//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

var shellBuiltins = map[string]struct{}{
	".": {}, ":": {}, "[": {}, "alias": {}, "bg": {}, "bind": {}, "break": {}, "builtin": {},
	"cd": {}, "command": {}, "compgen": {}, "complete": {}, "continue": {}, "declare": {},
	"dirs": {}, "disown": {}, "echo": {}, "enable": {}, "eval": {}, "exec": {}, "exit": {},
	"export": {}, "false": {}, "fc": {}, "fg": {}, "getopts": {}, "hash": {}, "help": {},
	"history": {}, "jobs": {}, "kill": {}, "let": {}, "local": {}, "logout": {}, "mapfile": {},
	"popd": {}, "printf": {}, "pushd": {}, "pwd": {}, "read": {}, "readonly": {}, "return": {},
	"set": {}, "shift": {}, "shopt": {}, "source": {}, "suspend": {}, "test": {}, "times": {},
	"trap": {}, "true": {}, "type": {}, "typeset": {}, "ulimit": {}, "umask": {}, "unalias": {},
	"unset": {}, "wait": {},
}

func looksLikeTerminalControlSequence(raw string) bool {
	if raw == "" {
		return false
	}
	if raw[0] == 0x1b {
		return true
	}
	return strings.Contains(raw, "\x1b[") ||
		strings.Contains(raw, "\x1b]") ||
		strings.Contains(raw, "\x1bP") ||
		strings.Contains(raw, "\x9b") ||
		strings.Contains(raw, "\x90")
}

func shouldIssuePastedChunk(detector *SecretDetector, raw string) bool {
	if strings.Contains(raw, "VK:") || strings.Contains(raw, "VE:") {
		return false
	}
	if looksLikeTerminalControlSequence(raw) {
		return false
	}
	core := strings.TrimSpace(strings.TrimRight(raw, "\r\n"))
	if core == "" {
		return false
	}
	fields := strings.Fields(core)
	if len(fields) == 0 {
		return false
	}
	if _, ok := shellBuiltins[fields[0]]; ok {
		return false
	}
	if _, err := exec.LookPath(fields[0]); err == nil {
		return false
	}
	return detector.ProcessLine(raw) == raw
}

func transformPastedInput(detector *SecretDetector, data []byte) string {
	raw := string(data)
	action := os.Getenv("VEILKEY_PLAINTEXT_ACTION")
	processed := detector.ProcessLine(raw)
	if action == "" || !strings.HasPrefix(action, "issue-temp") || !pasteTempIssuanceEnabled() {
		return processed
	}
	if processed != raw {
		return processed
	}
	if !shouldIssuePastedChunk(detector, raw) {
		return raw
	}

	core := strings.TrimRight(raw, "\r\n")
	suffix := raw[len(core):]
	vk := detector.issueVeilKey(core)
	if vk == "" {
		return raw
	}
	if detector.logger != nil {
		preview := core
		if len(preview) > 4 {
			preview = preview[:4] + "***"
		} else {
			preview = "***"
		}
		detector.logger.Log(vk, "paste", 100, preview)
	}
	detector.Stats.Detections++
	return vk + suffix
}

// processStreamPty — PTY output stream processor
// Complete lines (\n) are immediately filtered; partial lines are buffered for 30ms then watchlist-matched
// This ensures watchlist values are masked even in typing echo
func processStreamPty(detector *SecretDetector, r io.Reader, w io.Writer) {
	dataCh := make(chan []byte, 64)

	// PTY reader goroutine
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				dataCh <- chunk
			}
			if err != nil {
				close(dataCh)
				return
			}
		}
	}()

	var partialBuf bytes.Buffer
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	flushPartial := func() {
		if partialBuf.Len() == 0 {
			return
		}
		data := partialBuf.String()
		partialBuf.Reset()
		if len(detector.watchlist) > 0 {
			data = detector.ProcessLine(data)
		}
		w.Write([]byte(data))
	}

	for {
		select {
		case chunk, ok := <-dataCh:
			if !ok {
				flushPartial()
				return
			}

			// Stop existing timer (merge into partial buffer)
			timer.Stop()

			data := chunk
			lastNL := bytes.LastIndexByte(data, '\n')

			if lastNL >= 0 {
				// Has \n — process partial buffer + complete lines immediately
				partialBuf.Write(data[:lastNL+1])
				lines := bytes.Split(partialBuf.Bytes(), []byte{'\n'})
				partialBuf.Reset()
				// Skip last empty split
				for i, line := range lines {
					if i == len(lines)-1 && len(line) == 0 {
						break
					}
					processed := detector.ProcessLine(string(line))
					w.Write([]byte(processed))
					w.Write([]byte{'\n'})
				}
				// Store remainder after lastNL in buffer
				if lastNL+1 < len(data) {
					partialBuf.Write(data[lastNL+1:])
					timer.Reset(30 * time.Millisecond)
				}
			} else {
				// No \n — accumulate in partial buffer and start timer
				partialBuf.Write(data)
				timer.Reset(30 * time.Millisecond)
			}

		case <-timer.C:
			flushPartial()
		}
	}
}

// filterStdinToPty — Input filter: timer-based paste detection
// After key input, wait 5ms — if no more data, forward as keypress
// If more data arrives within 5ms — treat as paste, filter secrets then forward
// On Enter: match watchlist and replace visible input text with masked version
func filterStdinToPty(detector *SecretDetector, stdin io.Reader, ptmx io.Writer) {
	dataCh := make(chan []byte, 16)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				dataCh <- chunk
			}
			if err != nil {
				close(dataCh)
				return
			}
		}
	}()

	var inputBuf bytes.Buffer
	var lineBuf bytes.Buffer // Track current line (for Enter-time masking)
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	// maskLine — Replace visible input text on screen with masked version
	maskLine := func() {
		lineStr := lineBuf.String()
		masked := lineStr
		for _, w := range detector.watchlist {
			if strings.Contains(masked, w.Value) {
				masked = strings.ReplaceAll(masked, w.Value, w.VK)
			}
		}
		if masked != lineStr {
			// Erase input text with backspaces → clear line → write masked version
			n := len(lineStr)
			erase := bytes.Repeat([]byte{'\b'}, n)
			os.Stdout.Write(erase)
			os.Stdout.Write([]byte("\033[K"))
			os.Stdout.Write([]byte(masked))
		}
	}

	flush := func() {
		if inputBuf.Len() == 0 {
			return
		}
		data := inputBuf.Bytes()
		inputBuf.Reset()

		if len(data) > 4 {
			// Paste — filter secrets and issue temp refs only for non-command payloads.
			processed := transformPastedInput(detector, data)
			ptmx.Write([]byte(processed))
		} else {
			// Single key input
			if len(detector.watchlist) > 0 && !detector.Paused {
				for _, b := range data {
					switch {
					case b == '\r' || b == '\n':
						maskLine()
						lineBuf.Reset()
					case b == 127 || b == 8:
						if lineBuf.Len() > 0 {
							lineBuf.Truncate(lineBuf.Len() - 1)
						}
					case b == 3 || b == 21: // Ctrl+C, Ctrl+U
						lineBuf.Reset()
					case b == 27: // ESC — start of arrow key sequence
						lineBuf.Reset()
					case b >= 32:
						lineBuf.WriteByte(b)
					}
				}
			}
			ptmx.Write(data)
		}
	}

	for {
		select {
		case chunk, ok := <-dataCh:
			if !ok {
				flush()
				return
			}
			inputBuf.Write(chunk)
			timer.Reset(5 * time.Millisecond)

		case <-timer.C:
			flush()
		}
	}
}

func cmdWrapPty(args []string, apiURL, logPath, patternsFile string) {
	if len(args) == 0 {
		args = []string{"bash"}
	}

	detector := makeDetector(apiURL, logPath, patternsFile, false)

	// Save PID file (used by vk script to send SIGUSR1)
	stateDir := os.Getenv("VEILKEY_STATE_DIR")
	if stateDir == "" {
		stateDir = defaultStateDir()
	}
	os.MkdirAll(stateDir, 0755)
	pidPath := stateDir + "/guard.pid"
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	defer os.Remove(pidPath)

	c := exec.Command(args[0], args[1:]...)
	c.Env = append(os.Environ(), "TERM="+os.Getenv("TERM"))
	ptmx, err := pty.Start(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to start pty: %v\n", err)
		os.Exit(1)
	}
	defer ptmx.Close()

	// SIGUSR2 → pause watchlist matching (while vk is running)
	// SIGUSR1 → reload watchlist + resume matching
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGUSR1:
				detector.Paused = false
				detector.ReloadWatchlist()
			case syscall.SIGUSR2:
				detector.Paused = true
			}
		}
	}()

	// Handle terminal resize signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH // Set initial size

	// Switch stdin to raw mode (key input forwarded immediately)
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: failed to set raw mode: %v\n", err)
		oldState = nil
	}

	// stdin → secret detection → pty (replace secrets on paste)
	go func() {
		filterStdinToPty(detector, os.Stdin, ptmx)
	}()

	// pty → processStreamPty → stdout (zero-latency filtering)
	processStreamPty(detector, ptmx, os.Stdout)

	// Wait for process to exit
	exitCode := 0
	if err := c.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Restore raw mode (must restore before printing messages for proper line breaks)
	if oldState != nil {
		term.Restore(int(os.Stdin.Fd()), oldState)
	}

	if detector.Stats.Detections > 0 {
		fmt.Fprintf(errWriter, "\n[veilkey-cli] %d secret(s) detected and replaced\n", detector.Stats.Detections)
	}

	os.Exit(exitCode)
}
