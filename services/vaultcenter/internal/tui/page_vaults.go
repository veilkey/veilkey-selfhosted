package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type vaultTab int

const (
	vaultTabList vaultTab = iota
	vaultTabAgents
	vaultTabCatalog
)

type vaultsModel struct {
	tab     vaultTab
	loading bool
	offline bool

	// Vault list
	vaults []map[string]any
	cursor int

	// Vault detail → secrets
	showDetail     bool
	detailVault    map[string]any
	secrets        []map[string]any
	secretsCursor  int
	secretsLoading bool

	// Secret detail
	showSecretDetail bool
	secretDetail     map[string]any
	secretMeta       map[string]any
	secretBindings   []map[string]any
	metaLoading      bool
	revealValue      string
	revealing        bool

	// Agents
	agents      []map[string]any
	agentCursor int

	// Catalog
	catalog      []map[string]any
	catalogCursor int
}

type Vault = map[string]any
type VaultSecret = map[string]any

type vaultsLoadedMsg struct{ vaults []map[string]any }
type secretsLoadedMsg struct{ secrets []map[string]any }
type agentsLoadedMsg struct{ agents []map[string]any }
type catalogLoadedMsg struct{ catalog []map[string]any }
type secretMetaMsg struct {
	meta     map[string]any
	bindings []map[string]any
}
type secretRevealedMsg struct{ value string }

func newVaultsModel() vaultsModel {
	return vaultsModel{loading: true}
}

func loadVaultsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		vaults, err := c.ListVaults()
		if err != nil {
			return errMsg{err}
		}
		return vaultsLoadedMsg{vaults}
	}
}

func loadSecretsCmd(c *Client, vaultHash string) tea.Cmd {
	return func() tea.Msg {
		keys, err := c.GetVaultKeys(vaultHash)
		if err != nil {
			return errMsg{err}
		}
		return secretsLoadedMsg{keys}
	}
}

func loadAgentsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		agents, err := c.ListAgents()
		if err != nil {
			return errMsg{err}
		}
		return agentsLoadedMsg{agents}
	}
}

func loadCatalogCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		catalog, err := c.ListSecretCatalog()
		if err != nil {
			return errMsg{err}
		}
		return catalogLoadedMsg{catalog}
	}
}

func revealSecretCmd(c *Client, ref string) tea.Cmd {
	return func() tea.Msg {
		// Authorize first, then reveal
		if err := c.RevealAuthorize(ref, "TUI admin reveal"); err != nil {
			return errMsg{err}
		}
		val, err := c.RevealSecret(ref)
		if err != nil {
			return errMsg{err}
		}
		return secretRevealedMsg{val}
	}
}

func loadSecretMetaCmd(c *Client, vaultHash, name string) tea.Cmd {
	return func() tea.Msg {
		meta, _ := c.GetSecretMeta(vaultHash, name)
		bindings, _ := c.GetSecretBindings(vaultHash, name)
		return secretMetaMsg{meta, bindings}
	}
}

func (m vaultsModel) update(msg tea.Msg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case vaultsLoadedMsg:
		m.vaults = msg.vaults
		m.loading = false
		m.offline = false
		m.cursor = clampCursor(m.cursor, len(m.vaults))
		return m, nil

	case secretsLoadedMsg:
		m.secrets = msg.secrets
		m.secretsLoading = false
		m.secretsCursor = clampCursor(m.secretsCursor, len(m.secrets))
		return m, nil

	case agentsLoadedMsg:
		m.agents = msg.agents
		m.loading = false
		m.agentCursor = clampCursor(m.agentCursor, len(m.agents))
		return m, nil

	case catalogLoadedMsg:
		m.catalog = msg.catalog
		m.loading = false
		m.catalogCursor = clampCursor(m.catalogCursor, len(m.catalog))
		return m, nil

	case secretMetaMsg:
		m.secretMeta = msg.meta
		m.secretBindings = msg.bindings
		m.metaLoading = false
		return m, nil

	case secretRevealedMsg:
		m.revealValue = msg.value
		m.revealing = false
		return m, nil

	case errMsg:
		m.loading = false
		m.offline = true
		return m, nil

	case tea.KeyMsg:
		// Tab switching
		if !m.showDetail && !m.showSecretDetail {
			switch msg.String() {
			case "tab":
				switch m.tab {
				case vaultTabList:
					m.tab = vaultTabAgents
					m.loading = true
					return m, loadAgentsCmd(c)
				case vaultTabAgents:
					m.tab = vaultTabCatalog
					m.loading = true
					return m, loadCatalogCmd(c)
				case vaultTabCatalog:
					m.tab = vaultTabList
				}
				return m, nil
			}
		}

		if m.showSecretDetail {
			switch msg.String() {
			case "r":
				if !m.revealing && m.revealValue == "" {
					ref := str(m.secretDetail, "token")
					if ref == "" {
						ref = str(m.secretDetail, "ref")
					}
					m.revealing = true
					return m, revealSecretCmd(c, ref)
				}
			case "h":
				m.revealValue = ""
			case "esc":
				m.showSecretDetail = false
				m.revealValue = ""
				m.revealing = false
			}
			return m, nil
		}
		if m.showDetail {
			return m.updateSecrets(msg, c)
		}

		switch m.tab {
		case vaultTabList:
			return m.updateList(msg, c)
		case vaultTabAgents:
			return m.updateAgents(msg, c)
		case vaultTabCatalog:
			return m.updateCatalog(msg)
		}
	}
	return m, nil
}

func (m vaultsModel) updateList(msg tea.KeyMsg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.vaults)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.vaults) > 0 {
			m.showDetail = true
			m.detailVault = m.vaults[m.cursor]
			m.secretsLoading = true
			m.secretsCursor = 0
			return m, loadSecretsCmd(c, str(m.detailVault, "vault_runtime_hash"))
		}
	case "r":
		m.loading = true
		return m, loadVaultsCmd(c)
	}
	return m, nil
}

func (m vaultsModel) updateSecrets(msg tea.KeyMsg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.secretsCursor < len(m.secrets)-1 {
			m.secretsCursor++
		}
	case "k", "up":
		if m.secretsCursor > 0 {
			m.secretsCursor--
		}
	case "enter":
		if len(m.secrets) > 0 {
			s := m.secrets[m.secretsCursor]
			m.showSecretDetail = true
			m.secretDetail = s
			m.metaLoading = true
			return m, loadSecretMetaCmd(c, str(m.detailVault, "vault_runtime_hash"), str(s, "name"))
		}
	case "esc":
		m.showDetail = false
	case "r":
		m.secretsLoading = true
		return m, loadSecretsCmd(c, str(m.detailVault, "vault_runtime_hash"))
	}
	return m, nil
}

func (m vaultsModel) updateAgents(msg tea.KeyMsg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.agentCursor < len(m.agents)-1 {
			m.agentCursor++
		}
	case "k", "up":
		if m.agentCursor > 0 {
			m.agentCursor--
		}
	case "r":
		m.loading = true
		return m, loadAgentsCmd(c)
	}
	return m, nil
}

func (m vaultsModel) updateCatalog(msg tea.KeyMsg) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.catalogCursor < len(m.catalog)-1 {
			m.catalogCursor++
		}
	case "k", "up":
		if m.catalogCursor > 0 {
			m.catalogCursor--
		}
	}
	return m, nil
}

func (m vaultsModel) view(width int) string {
	if m.showSecretDetail {
		return m.viewSecretDetail()
	}
	if m.showDetail {
		return m.viewSecrets(width)
	}

	// Sub-tabs
	tabs := []struct {
		name string
		t    vaultTab
	}{
		{"Vaults", vaultTabList},
		{"Agents", vaultTabAgents},
		{"Catalog", vaultTabCatalog},
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

	switch m.tab {
	case vaultTabAgents:
		return header + m.viewAgents(width)
	case vaultTabCatalog:
		return header + m.viewCatalog(width)
	default:
		return header + m.viewList(width)
	}
}

func (m vaultsModel) viewList(width int) string {
	var b strings.Builder

	if m.loading {
		b.WriteString(styleDim.Render("  Loading..."))
		return b.String()
	}
	if m.offline {
		b.WriteString(styleError.Render("  ⚠ Cannot reach VaultCenter"))
		return b.String()
	}
	if len(m.vaults) == 0 {
		b.WriteString(styleDim.Render("  No vaults."))
		return b.String()
	}

	h := fmt.Sprintf("  %-20s %-20s %-10s %-10s %-10s", "Name", "Display", "Status", "Mode", "Blocked")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, v := range m.vaults {
		name := str(v, "vault_name")
		if name == "" {
			name = str(v, "display_name")
		}
		line := fmt.Sprintf("  %-20s %-20s %-10s %-10s %-10s",
			truncate(name, 18),
			truncate(str(v, "display_name"), 18),
			str(v, "status"),
			str(v, "mode"),
			str(v, "blocked"),
		)
		if i == m.cursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  enter secrets  tab switch  r refresh"))
	return b.String()
}

func (m vaultsModel) viewSecrets(width int) string {
	var b strings.Builder
	name := str(m.detailVault, "vault_name")
	if name == "" {
		name = str(m.detailVault, "vault_hash")
	}
	b.WriteString(styleHeader.Render(fmt.Sprintf("  %s — Secrets", name)))
	b.WriteString("\n\n")

	if m.secretsLoading {
		b.WriteString(styleDim.Render("  Loading..."))
		return b.String()
	}
	if len(m.secrets) == 0 {
		b.WriteString(styleDim.Render("  No secrets."))
		b.WriteString("\n\n" + styleDim.Render("  esc back"))
		return b.String()
	}

	h := fmt.Sprintf("  %-28s %-28s %-10s %-10s", "Name", "Ref", "Scope", "Status")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, s := range m.secrets {
		ref := str(s, "token")
		if ref == "" {
			ref = str(s, "ref")
		}
		line := fmt.Sprintf("  %-28s %-28s %-10s %-10s",
			truncate(str(s, "name"), 26),
			truncate(ref, 26),
			str(s, "scope"),
			str(s, "status"),
		)
		if i == m.secretsCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  enter detail  r refresh  esc back"))
	return b.String()
}

func (m vaultsModel) viewSecretDetail() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("  Secret: %s", str(m.secretDetail, "name"))))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString("  ")
		b.WriteString(styleLabel.Render(label))
		b.WriteString(styleValue.Render(value))
		b.WriteString("\n")
	}
	ref := str(m.secretDetail, "token")
	if ref == "" {
		ref = str(m.secretDetail, "ref")
	}
	row("Name", str(m.secretDetail, "name"))
	row("Ref", ref)
	row("Scope", str(m.secretDetail, "scope"))
	row("Status", str(m.secretDetail, "status"))
	row("Version", str(m.secretDetail, "version"))

	if m.metaLoading {
		b.WriteString("\n  " + styleDim.Render("Loading metadata..."))
	} else {
		if m.secretMeta != nil {
			b.WriteString("\n")
			for k, v := range m.secretMeta {
				if k != "name" && k != "ref" {
					row(k, fmt.Sprintf("%v", v))
				}
			}
		}

		if len(m.secretBindings) > 0 {
			b.WriteString("\n  " + styleHeader.Render("Bindings") + "\n")
			for _, bind := range m.secretBindings {
				b.WriteString(fmt.Sprintf("    %s → %s (%s)\n",
					str(bind, "binding_type"),
					str(bind, "target_name"),
					str(bind, "binding_id"),
				))
			}
		}
	}

	// Reveal — VK: masked, VE: shown as-is
	b.WriteString("\n")
	isVK := strings.HasPrefix(ref, "VK:")
	if isVK {
		if m.revealing {
			b.WriteString("  " + styleDim.Render("Decrypting..."))
		} else if m.revealValue != "" {
			b.WriteString("  " + styleLabel.Render("Value"))
			b.WriteString(styleReveal.Render(m.revealValue))
			b.WriteString("\n  " + styleDim.Render("h hide"))
		} else {
			b.WriteString("  " + styleLabel.Render("Value"))
			b.WriteString(styleDim.Render("••••••••"))
			b.WriteString("\n  " + styleDim.Render("r reveal"))
		}
	} else {
		// VE: refs — show ref as value (not encrypted by VaultCenter)
		b.WriteString("  " + styleLabel.Render("Value"))
		b.WriteString(styleValue.Render(ref))
	}

	b.WriteString("\n\n")
	if isVK {
		b.WriteString(styleDim.Render("  r reveal  h hide  esc back"))
	} else {
		b.WriteString(styleDim.Render("  esc back"))
	}
	return b.String()
}

func (m vaultsModel) viewAgents(width int) string {
	var b strings.Builder

	if m.loading {
		b.WriteString(styleDim.Render("  Loading..."))
		return b.String()
	}
	if len(m.agents) == 0 {
		b.WriteString(styleDim.Render("  No agents."))
		return b.String()
	}

	h := fmt.Sprintf("  %-18s %-10s %-10s %-8s %-8s %-8s", "Vault Name", "Status", "Health", "Ver", "Secrets", "Rotate")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, a := range m.agents {
		vname := str(a, "vault_name")
		if vname == "" {
			vname = truncate(str(a, "vault_hash"), 16)
		}
		line := fmt.Sprintf("  %-18s %-10s %-10s %-8s %-8s %-8s",
			truncate(vname, 16),
			str(a, "status"),
			str(a, "health"),
			str(a, "key_version"),
			str(a, "secrets_count"),
			str(a, "rotation_required"),
		)
		if i == m.agentCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  tab switch  r refresh"))
	return b.String()
}

func (m vaultsModel) viewCatalog(width int) string {
	var b strings.Builder

	if m.loading {
		b.WriteString(styleDim.Render("  Loading..."))
		return b.String()
	}
	if len(m.catalog) == 0 {
		b.WriteString(styleDim.Render("  No secrets in catalog."))
		return b.String()
	}

	h := fmt.Sprintf("  %-24s %-20s %-14s %-10s", "Name", "Ref", "Class", "Bindings")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, s := range m.catalog {
		line := fmt.Sprintf("  %-24s %-20s %-14s %-10s",
			truncate(str(s, "secret_name"), 22),
			truncate(str(s, "ref_canonical"), 18),
			str(s, "class"),
			str(s, "binding_count"),
		)
		if i == m.catalogCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  tab switch"))
	return b.String()
}

func clampCursor(cursor, length int) int {
	if cursor >= length {
		return max(0, length-1)
	}
	return cursor
}
