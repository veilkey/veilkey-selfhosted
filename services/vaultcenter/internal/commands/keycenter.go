package commands

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"veilkey-vaultcenter/internal/tui"
)

func RunKeycenter() {
	addr := os.Getenv("VEILKEY_KEYCENTER_URL")
	if addr == "" {
		addr = os.Getenv("VEILKEY_LOCALVAULT_URL")
	}
	if addr == "" {
		addr = os.Getenv("VEILKEY_ADDR")
	}
	if addr == "" {
		log.Fatal("VEILKEY_KEYCENTER_URL, VEILKEY_LOCALVAULT_URL, or VEILKEY_ADDR is required")
	}

	// Normalize to a full URL
	if strings.HasPrefix(addr, ":") {
		addr = "https://localhost" + addr
	}
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}

	if _, err := url.Parse(addr); err != nil {
		log.Fatalf("Invalid URL: %s (%v)", addr, err)
	}

	fmt.Printf("Connecting to VaultCenter at %s...\n", addr)

	p := tea.NewProgram(
		tui.NewModel(addr),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}
