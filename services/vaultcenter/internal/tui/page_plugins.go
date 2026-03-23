package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pluginsModel struct {
	plugins []pluginItem
	loaded  bool
	err     string
}

type pluginItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Loaded      bool   `json:"loaded"`
	WasmFile    string `json:"wasm_file"`
	InstalledAt string `json:"installed_at"`
}

type pluginsListMsg struct {
	plugins []pluginItem
	err     error
}

func newPluginsModel() pluginsModel {
	return pluginsModel{}
}

func fetchPluginsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		result, err := c.getJSON("/api/plugins")
		if err != nil {
			return pluginsListMsg{err: err}
		}
		// Parse plugins from result
		raw, _ := json.Marshal(result["plugins"])
		var plugins []pluginItem
		json.Unmarshal(raw, &plugins)
		return pluginsListMsg{plugins: plugins}
	}
}

func (m pluginsModel) update(msg tea.Msg, c *Client) (pluginsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case loginSuccessMsg:
		return m, fetchPluginsCmd(c)
	case pluginsListMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.plugins = msg.plugins
			m.loaded = true
			m.err = ""
		}
	case tea.KeyMsg:
		if msg.String() == "r" {
			return m, fetchPluginsCmd(c)
		}
	}
	return m, nil
}

func (m pluginsModel) view(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Render("Plugins")
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(styleError.Render("Error: " + m.err) + "\n")
	}

	if !m.loaded {
		b.WriteString("  Loading...\n")
		return b.String()
	}

	if len(m.plugins) == 0 {
		b.WriteString("  No plugins installed.\n")
		b.WriteString("\n  Install: POST /api/plugins (multipart: wasm + manifest)\n")
		return b.String()
	}

	// Header
	header := fmt.Sprintf("  %-20s %-10s %-8s %-40s", "NAME", "VERSION", "LOADED", "DESCRIPTION")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(header) + "\n")
	b.WriteString(strings.Repeat("─", min(width, 80)) + "\n")

	for _, p := range m.plugins {
		loadedStr := "○"
		loadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		if p.Loaded {
			loadedStr = "●"
			loadedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		}
		desc := p.Description
		if len(desc) > 38 {
			desc = desc[:35] + "..."
		}
		line := fmt.Sprintf("  %-20s %-10s %s  %-40s",
			p.Name, p.Version, loadedStyle.Render(loadedStr), desc)
		b.WriteString(line + "\n")
	}

	b.WriteString("\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("[r] refresh") + "\n")
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
