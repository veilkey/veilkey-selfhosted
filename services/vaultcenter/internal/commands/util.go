package commands

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func readPasswordFromFileEnv() string {
	path := os.Getenv("VEILKEY_PASSWORD_FILE")
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read VEILKEY_PASSWORD_FILE (%s): %v", path, err)
	}
	pw := strings.TrimRight(string(data), "\n\r")
	if pw == "" {
		log.Fatalf("VEILKEY_PASSWORD_FILE (%s) is empty", path)
	}
	return pw
}

func readPassword(prompt string) string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		return ""
	}

	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Fprint(os.Stderr, prompt)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		return ""
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func parseDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("warning: invalid duration %s=%q, using default %s", key, v, defaultVal)
	}
	return defaultVal
}

func detectExternalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func generateInitRef(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
