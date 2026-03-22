package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type settingsTab int

const (
	settingsStatus settingsTab = iota
	settingsSecurity
	settingsTokens
	settingsConfigs
)

type settingsModel struct {
	tab     settingsTab
	loading bool
	offline bool

	status   map[string]any
	nodeInfo map[string]any
	authInfo map[string]any

	tokens      []map[string]any
	tokenCursor int

	configs      []map[string]any
	configCursor int

	// Create token
	creatingToken bool
	tokenInput    textinput.Model

	// Action feedback
	message string
}

type settingsLoadedMsg struct {
	status   map[string]any
	nodeInfo map[string]any
	authInfo map[string]any
}
type tokensLoadedMsg struct{ tokens []map[string]any }
type configsLoadedMsg struct{ configs []map[string]any }
type tokenCreatedMsg struct{ tokenID string }
type tokenRevokedMsg struct{}
type rotationScheduledMsg struct{}

func newSettingsModel() settingsModel {
	ti := textinput.New()
	ti.Placeholder = "token label"
	ti.CharLimit = 64
	ti.Width = 40
	return settingsModel{loading: true, tokenInput: ti}
}

func loadSettingsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		status, _ := c.Status()
		nodeInfo, _ := c.GetNodeInfo()
		auth, _ := c.AuthSettings()
		return settingsLoadedMsg{status, nodeInfo, auth}
	}
}

func loadTokensCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		tokens, err := c.ListRegistrationTokens()
		if err != nil {
			return errMsg{err}
		}
		return tokensLoadedMsg{tokens}
	}
}

func loadConfigsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		configs, err := c.ListConfigs()
		if err != nil {
			return errMsg{err}
		}
		return configsLoadedMsg{configs}
	}
}

func createTokenCmd(c *Client, label string) tea.Cmd {
	return func() tea.Msg {
		data, err := c.CreateRegistrationToken(label)
		if err != nil {
			return errMsg{err}
		}
		return tokenCreatedMsg{str(data, "token_id")}
	}
}

func revokeTokenCmd(c *Client, tokenID string) tea.Cmd {
	return func() tea.Msg {
		if err := c.RevokeRegistrationToken(tokenID); err != nil {
			return errMsg{err}
		}
		return tokenRevokedMsg{}
	}
}

func scheduleRotationCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		_, err := c.ScheduleAllRotations()
		if err != nil {
			return errMsg{err}
		}
		return rotationScheduledMsg{}
	}
}

func (m settingsModel) update(msg tea.Msg, c *Client) (settingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case settingsLoadedMsg:
		m.status = msg.status
		m.nodeInfo = msg.nodeInfo
		m.authInfo = msg.authInfo
		m.loading = false
		m.offline = false
		return m, nil

	case tokensLoadedMsg:
		m.tokens = msg.tokens
		m.loading = false
		m.tokenCursor = clampCursor(m.tokenCursor, len(m.tokens))
		return m, nil

	case configsLoadedMsg:
		m.configs = msg.configs
		m.loading = false
		m.configCursor = clampCursor(m.configCursor, len(m.configs))
		return m, nil

	case tokenCreatedMsg:
		m.creatingToken = false
		m.message = "Token created: " + msg.tokenID
		return m, loadTokensCmd(c)

	case tokenRevokedMsg:
		m.message = "Token revoked"
		return m, loadTokensCmd(c)

	case rotationScheduledMsg:
		m.message = "Rotation scheduled for all agents"
		return m, nil

	case errMsg:
		m.loading = false
		m.offline = true
		return m, nil

	case tea.KeyMsg:
		if m.creatingToken {
			return m.updateCreateToken(msg, c)
		}

		switch msg.String() {
		case "tab":
			m.message = ""
			switch m.tab {
			case settingsStatus:
				m.tab = settingsSecurity
			case settingsSecurity:
				m.tab = settingsTokens
				m.loading = true
				return m, loadTokensCmd(c)
			case settingsTokens:
				m.tab = settingsConfigs
				m.loading = true
				return m, loadConfigsCmd(c)
			case settingsConfigs:
				m.tab = settingsStatus
			}
			return m, nil
		case "r":
			m.loading = true
			m.message = ""
			switch m.tab {
			case settingsTokens:
				return m, loadTokensCmd(c)
			case settingsConfigs:
				return m, loadConfigsCmd(c)
			default:
				return m, loadSettingsCmd(c)
			}
		}

		switch m.tab {
		case settingsSecurity:
			if msg.String() == "R" { // Shift+R for rotate
				return m, scheduleRotationCmd(c)
			}
		case settingsTokens:
			return m.updateTokens(msg, c)
		case settingsConfigs:
			switch msg.String() {
			case "j", "down":
				if m.configCursor < len(m.configs)-1 {
					m.configCursor++
				}
			case "k", "up":
				if m.configCursor > 0 {
					m.configCursor--
				}
			}
		}
	}
	return m, nil
}

func (m settingsModel) updateTokens(msg tea.KeyMsg, c *Client) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.tokenCursor < len(m.tokens)-1 {
			m.tokenCursor++
		}
	case "k", "up":
		if m.tokenCursor > 0 {
			m.tokenCursor--
		}
	case "n":
		m.creatingToken = true
		m.tokenInput.SetValue("")
		m.tokenInput.Focus()
	case "d":
		if len(m.tokens) > 0 {
			tid := str(m.tokens[m.tokenCursor], "token_id")
			return m, revokeTokenCmd(c, tid)
		}
	}
	return m, nil
}

func (m settingsModel) updateCreateToken(msg tea.KeyMsg, c *Client) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		label := strings.TrimSpace(m.tokenInput.Value())
		if label == "" {
			return m, nil
		}
		return m, createTokenCmd(c, label)
	case "esc":
		m.creatingToken = false
		return m, nil
	}
	var cmd tea.Cmd
	m.tokenInput, cmd = m.tokenInput.Update(msg)
	return m, cmd
}

func (m settingsModel) view(width int) string {
	tabs := []struct {
		name string
		t    settingsTab
	}{
		{"Status", settingsStatus},
		{"Security", settingsSecurity},
		{"Tokens", settingsTokens},
		{"Configs", settingsConfigs},
	}
	var tabParts []string
	for _, t := range tabs {
		if t.t == m.tab {
			tabParts = append(tabParts, styleActive.Render(" "+t.name+" "))
		} else {
			tabParts = append(tabParts, styleInactive.Render(" "+t.name+" "))
		}
	}
	header := "  " + strings.Join(tabParts, " ") + "\n\n"

	if m.loading {
		return header + styleDim.Render("  Loading...")
	}
	if m.offline {
		return header + styleError.Render("  ⚠ Cannot reach VaultCenter")
	}

	var content string
	switch m.tab {
	case settingsStatus:
		content = m.viewStatus()
	case settingsSecurity:
		content = m.viewSecurity()
	case settingsTokens:
		content = m.viewTokens(width)
	case settingsConfigs:
		content = m.viewConfigs(width)
	}

	footer := "\n\n" + styleDim.Render("  tab switch section  r refresh")
	if m.message != "" {
		footer = "\n\n  " + lipgloss.NewStyle().Foreground(colorGreen).Render(m.message) + footer
	}

	return header + content + footer
}

func (m settingsModel) viewStatus() string {
	var b strings.Builder
	row := func(label string, value any) {
		b.WriteString("  ")
		b.WriteString(styleLabel.Render(label))
		b.WriteString(styleValue.Render(fmt.Sprintf("%v", value)))
		b.WriteString("\n")
	}

	if m.status != nil {
		row("Mode", m.status["mode"])
		row("Locked", m.status["locked"])
		row("Version", m.status["version"])
		row("Secrets", m.status["secrets_count"])
		row("Tracked Refs", m.status["tracked_refs_count"])
		row("Configs", m.status["configs_count"])
		row("Children", m.status["children_count"])
	}
	if m.nodeInfo != nil {
		b.WriteString("\n")
		row("Node ID", m.nodeInfo["node_id"])
		row("Parent URL", m.nodeInfo["parent_url"])
	}
	if features, ok := m.status["supported_features"].([]any); ok {
		var names []string
		for _, f := range features {
			names = append(names, fmt.Sprintf("%v", f))
		}
		row("Features", strings.Join(names, ", "))
	}
	return b.String()
}

func (m settingsModel) viewSecurity() string {
	var b strings.Builder
	row := func(label string, value any) {
		b.WriteString("  ")
		b.WriteString(styleLabel.Render(label))
		b.WriteString(styleValue.Render(fmt.Sprintf("%v", value)))
		b.WriteString("\n")
	}

	if m.authInfo != nil {
		row("TOTP Enabled", m.authInfo["totp_enabled"])
		row("TOTP Enrolled", m.authInfo["totp_enrolled"])
		row("Passkeys", m.authInfo["passkey_count"])
		row("Session TTL", fmt.Sprintf("%vs", m.authInfo["session_ttl_seconds"]))
		row("Idle Timeout", fmt.Sprintf("%vs", m.authInfo["idle_timeout_seconds"]))
		row("Reveal Window", fmt.Sprintf("%vs", m.authInfo["reveal_window_seconds"]))
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  Shift+R  Schedule rotation for all agents"))
	return b.String()
}

func (m settingsModel) viewTokens(width int) string {
	var b strings.Builder

	if m.creatingToken {
		b.WriteString(styleHeader.Render("  New Registration Token") + "\n\n")
		b.WriteString("  " + styleLabel.Render("Label") + "\n")
		b.WriteString("  " + m.tokenInput.View() + "\n\n")
		b.WriteString(styleDim.Render("  enter create  esc cancel"))
		return b.String()
	}

	if len(m.tokens) == 0 {
		b.WriteString(styleDim.Render("  No registration tokens."))
		b.WriteString("\n\n")
		b.WriteString(styleDim.Render("  n create token"))
		return b.String()
	}

	h := fmt.Sprintf("  %-20s %-34s %-10s", "Label", "Token ID", "Status")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, t := range m.tokens {
		line := fmt.Sprintf("  %-20s %-34s %-10s",
			truncate(str(t, "label"), 18),
			truncate(str(t, "token_id"), 32),
			str(t, "status"),
		)
		if i == m.tokenCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  n create  d revoke"))
	return b.String()
}

func (m settingsModel) viewConfigs(width int) string {
	var b strings.Builder

	if len(m.configs) == 0 {
		b.WriteString(styleDim.Render("  No configs."))
		return b.String()
	}

	h := fmt.Sprintf("  %-30s %-40s", "Key", "Value")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, cfg := range m.configs {
		line := fmt.Sprintf("  %-30s %-40s",
			truncate(str(cfg, "key"), 28),
			truncate(str(cfg, "value"), 38),
		)
		if i == m.configCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move"))
	return b.String()
}
