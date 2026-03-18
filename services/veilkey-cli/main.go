package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var version = "dev"

var errWriter io.Writer = os.Stderr

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultStateDir() string {
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	return tmpDir + "/veilkey-cli"
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  veilkey-cli scan [options] [file|-]   Scan file/stdin for secrets (detect only)
  veilkey-cli filter [file|-]           Replace secrets in file/stdin (stdout)
  veilkey-cli proxy [options]           Run the local egress proxy
  veilkey-cli wrap <command...>         Run command + auto-replace secrets
  veilkey-cli wrap-pty [command]        Interactive PTY + auto-replace (default: bash)
  veilkey-cli exec <command...>         Resolve VK: hashes + run command
  veilkey-cli resolve <VK:hash>         Resolve VK hash to original value
  veilkey-cli function <subcommand...>  Manage repo-tracked function wrappers
  veilkey-cli list                      List detected VeilKey entries
  veilkey-cli paste-mode [mode]         Get or set pasted temp issuance mode
  veilkey-cli clear                     Clear session log
  veilkey-cli status                    Show status
  veilkey-cli version                   Show version

Options:
  --format <text|json|sarif>   Output format (default: text)
  --config <path>              Config file path (default: .veilkey.yml)
  --exit-code                  Exit with 1 if secrets found (for CI)
  --patterns <path>            Custom patterns file

Environment:
  VEILKEY_LOCALVAULT_URL       Preferred localvault URL
  VEILKEY_API                  Legacy endpoint variable (fallback)
  VEILKEY_STATE_DIR            State directory (default: $TMPDIR/veilkey-cli)
  VEILKEY_FUNCTION_DIR         Function catalog directory for function subcommands
`)
}

func resolveAPIURL() string {
	if v := os.Getenv("VEILKEY_LOCALVAULT_URL"); v != "" {
		return v
	}
	if v := os.Getenv("VEILKEY_API"); v != "" {
		return v
	}
	return ""
}

func main() {
	apiURL := resolveAPIURL()
	if apiURL == "" {
		// These commands don't require API
		if len(os.Args) > 1 {
			switch os.Args[1] {
			case "version", "help", "-h", "--help", "scan", "list", "clear", "status", "proxy", "paste-mode":
				goto skipAPICheck
			case "function":
				if len(os.Args) > 2 {
					switch os.Args[2] {
					case "list", "show", "delete", "register", "test":
						goto skipAPICheck
					}
				}
			}
		}
		fmt.Fprintln(os.Stderr, "ERROR: VeilKey endpoint URL is required.")
		fmt.Fprintln(os.Stderr, "  export VEILKEY_LOCALVAULT_URL=<localvault-url>")
		os.Exit(1)
	}
skipAPICheck:
	stateDir := getEnv("VEILKEY_STATE_DIR", defaultStateDir())
	logPath := stateDir + "/session.log"
	os.MkdirAll(stateDir, 0755)

	var patternsFile, outputFormat, configPath string
	var exitCodeFlag bool
	args := os.Args[1:]

	// Extract flags (before subcommand parsing)
	var cleaned []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--patterns" && i+1 < len(args):
			patternsFile = args[i+1]
			i++
		case strings.HasPrefix(a, "--patterns="):
			patternsFile = strings.TrimPrefix(a, "--patterns=")
		case a == "--format" && i+1 < len(args):
			outputFormat = args[i+1]
			i++
		case strings.HasPrefix(a, "--format="):
			outputFormat = strings.TrimPrefix(a, "--format=")
		case a == "--config" && i+1 < len(args):
			configPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--config="):
			configPath = strings.TrimPrefix(a, "--config=")
		case a == "--exit-code":
			exitCodeFlag = true
		default:
			cleaned = append(cleaned, a)
		}
	}
	args = cleaned

	// Load project config (CLI flags override config file)
	projCfg, _ := LoadProjectConfig(configPath)
	if projCfg != nil {
		if patternsFile == "" && projCfg.PatternsFile != "" {
			patternsFile = projCfg.PatternsFile
		}
		if outputFormat == "" && projCfg.Format != "" {
			outputFormat = projCfg.Format
		}
		if !exitCodeFlag && projCfg.ExitCode {
			exitCodeFlag = true
		}
		if apiURL == "" && projCfg.APIURL != "" {
			apiURL = projCfg.APIURL
		}
	}
	if outputFormat == "" {
		outputFormat = "text"
	}

	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	cmd := args[0]

	switch cmd {
	case "wrap":
		cmdWrap(args[1:], apiURL, logPath, patternsFile)
	case "proxy":
		cmdProxy(args[1:])
	case "wrap-pty":
		cmdWrapPty(args[1:], apiURL, logPath, patternsFile)
	case "scan":
		file := "-"
		if len(args) > 1 {
			file = args[1]
		}
		cmdScan(file, apiURL, logPath, patternsFile, outputFormat, exitCodeFlag)
	case "filter":
		file := "-"
		if len(args) > 1 {
			file = args[1]
		}
		cmdFilter(file, apiURL, logPath, patternsFile)
	case "exec":
		cmdExec(args[1:], apiURL)
	case "resolve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: veilkey-cli resolve <VK:hash>")
			os.Exit(1)
		}
		cmdResolve(args[1], apiURL)
	case "list":
		cmdList(logPath)
	case "paste-mode":
		cmdPasteMode(args[1:])
	case "clear":
		cmdClear(logPath)
	case "function":
		cmdFunction(args[1:], apiURL)
	case "status":
		cmdStatus(apiURL, logPath, patternsFile)
	case "version":
		fmt.Printf("veilkey-cli %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		usage()
		os.Exit(1)
	}
}

func makeDetector(apiURL, logPath, patternsFile string, scanOnly bool) *SecretDetector {
	cfg, err := LoadConfig(patternsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	client := NewVeilKeyClient(apiURL)
	logger := NewSessionLogger(logPath)
	return NewSecretDetector(cfg, client, logger, scanOnly)
}

func processStream(detector *SecretDetector, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		processed := detector.ProcessLine(line)
		fmt.Println(processed)
	}
}

func cmdWrap(args []string, apiURL, logPath, patternsFile string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: veilkey-cli wrap <command...>")
		os.Exit(1)
	}

	detector := makeDetector(apiURL, logPath, patternsFile, false)

	c := exec.Command(args[0], args[1:]...)
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr

	stdout, err := c.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	processStream(detector, stdout)

	exitCode := 0
	if err := c.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if detector.Stats.Detections > 0 {
		fmt.Fprintf(os.Stderr, "\n[veilkey-cli] %d secret(s) detected and replaced\n", detector.Stats.Detections)
	}

	os.Exit(exitCode)
}

func cmdResolve(hash, apiURL string) {
	client := NewVeilKeyClient(apiURL)
	value, err := client.Resolve(hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(value)
}

func cmdExec(args []string, apiURL string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: veilkey-cli exec <command...>")
		os.Exit(1)
	}

	client := NewVeilKeyClient(apiURL)

	// Resolve VK: hashes in arguments
	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = veilkeyRE.ReplaceAllStringFunc(arg, func(hash string) string {
			val, err := client.Resolve(hash)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: resolve %s failed: %v\n", hash, err)
				return hash
			}
			return val
		})
	}

	c := exec.Command(resolved[0], resolved[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func cmdScan(file, apiURL, logPath, patternsFile, outputFormat string, exitCode bool) {
	detector := makeDetector(apiURL, logPath, patternsFile, true)
	formatter := NewFormatter(outputFormat, os.Stdout)

	var r io.Reader
	var fileName string
	if file == "-" {
		r = os.Stdin
		fileName = "<stdin>"
	} else {
		f, err := os.Open(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: file not found: %s\n", file)
			os.Exit(1)
		}
		defer f.Close()
		r = f
		fileName = file
	}

	formatter.Header()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		detections := detector.DetectSecrets(line)
		detector.Stats.Lines++
		for _, det := range detections {
			detector.Stats.Detections++
			preview := det.Value
			if len(preview) > 8 {
				preview = preview[:8] + "***"
			}
			formatter.FormatFinding(Finding{
				File:       fileName,
				Line:       lineNum,
				Pattern:    det.Pattern,
				Confidence: det.Confidence,
				Match:      preview,
			})
		}
	}

	formatter.FormatSummary(detector.Stats)
	formatter.Footer()

	if exitCode && detector.Stats.Detections > 0 {
		os.Exit(1)
	}
}

func cmdFilter(file, apiURL, logPath, patternsFile string) {
	detector := makeDetector(apiURL, logPath, patternsFile, false) // scanOnly=false → replace with VK:hash

	var r io.Reader
	if file == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: file not found: %s\n", file)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	processStream(detector, r)

	if detector.Stats.Detections > 0 {
		fmt.Fprintf(os.Stderr, "\n[veilkey-cli] %d secret(s) detected and replaced\n", detector.Stats.Detections)
	}
}

func cmdList(logPath string) {
	logger := NewSessionLogger(logPath)
	entries, err := logger.ReadEntries()
	if err != nil || len(entries) == 0 {
		fmt.Println("No secrets detected")
		return
	}

	fmt.Printf("\033[0;36m%-20s %-25s %-8s %s\033[0m\n", "VEILKEY", "PATTERN", "CONF", "TIMESTAMP")
	fmt.Println("────────────────────────────────────────────────────────────────────")

	for _, e := range entries {
		fmt.Printf("\033[0;32m%-20s\033[0m %-25s %-8d %s\n", e.VeilKey, e.Pattern, e.Confidence, e.Timestamp)
	}

	fmt.Printf("\nTotal: %d VeilKey(s)\n", len(entries))
}

func cmdClear(logPath string) {
	logger := NewSessionLogger(logPath)
	logger.Clear()
	fmt.Println("Session log cleared")
}

func cmdPasteMode(args []string) {
	if len(args) == 0 || args[0] == "status" {
		fmt.Printf("paste-mode: %s\n", currentPasteMode())
		return
	}
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "Usage: veilkey-cli paste-mode [on|off|status]")
		os.Exit(1)
	}
	if err := setPasteMode(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("paste-mode: %s\n", currentPasteMode())
}

func cmdStatus(apiURL, logPath, patternsFile string) {
	cfg, err := LoadConfig(patternsFile)

	fmt.Println("\033[0;36m=== veilkey-cli ===\033[0m")
	fmt.Println()
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("API:     %s\n", apiURL)
	fmt.Printf("Log:     %s\n", logPath)
	fmt.Printf("Paste:   %s\n", currentPasteMode())
	fmt.Println()

	logger := NewSessionLogger(logPath)
	fmt.Printf("Secrets: %d detected\n", logger.Count())

	client := NewVeilKeyClient(apiURL)
	if client.HealthCheck() {
		fmt.Println("API:     \033[0;32mconnected\033[0m")
	} else {
		fmt.Println("API:     \033[0;31munreachable\033[0m")
	}

	if err == nil {
		fmt.Printf("Patterns: %d loaded\n", len(cfg.Patterns))
	}
}
