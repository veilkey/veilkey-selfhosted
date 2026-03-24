package tui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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
	showDetail      bool
	detailVault     map[string]any
	secrets         []map[string]any
	filteredSecrets []map[string]any
	secretsCursor   int
	secretsLoading  bool

	// Search
	searching   bool
	searchInput textinput.Model
	searchQuery string

	// Secret create/edit
	creatingSecret bool
	editingSecret  bool
	editSecretName string
	createName     textinput.Model
	createValue    textinput.Model
	createFocus    int

	// Secret detail
	showSecretDetail bool
	secretDetail     map[string]any
	secretMeta       map[string]any
	secretBindings   []map[string]any
	metaLoading      bool
	revealValue      string
	revealing        bool

	// Delete confirm
	confirmDelete    bool
	deleteTargetName string

	// Agents
	agents      []map[string]any
	agentCursor int

	// Catalog
	catalog          []map[string]any
	filteredCatalog  []map[string]any
	catalogCursor    int
	catalogSearching bool
	catalogSearch    textinput.Model
	catalogQuery     string
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

type secretCreatedMsg struct{}
type secretDeletedMsg struct{}

func newVaultsModel() vaultsModel {
	si := textinput.New()
	si.Placeholder = "search ref or name..."
	si.Width = 40

	ci := textinput.New()
	ci.Placeholder = "search..."
	ci.Width = 40

	cn := textinput.New()
	cn.Placeholder = "SECRET_NAME (A-Z_)"
	cn.CharLimit = 128
	cn.Width = 40

	cv := textinput.New()
	cv.Placeholder = "plaintext value"
	cv.CharLimit = 4096
	cv.Width = 40
	cv.EchoMode = textinput.EchoPassword
	cv.EchoCharacter = '•'

	return vaultsModel{
		loading:       true,
		searchInput:   si,
		catalogSearch: ci,
		createName:    cn,
		createValue:   cv,
	}
}

func createSecretCmd(c *Client, runtimeHash, name, value string) tea.Cmd {
	return func() tea.Msg {
		_, err := c.CreateVaultSecret(runtimeHash, name, value)
		if err != nil {
			return errMsg{err}
		}
		return secretCreatedMsg{}
	}
}

func updateSecretCmd(c *Client, runtimeHash, name, value string) tea.Cmd {
	return func() tea.Msg {
		_, err := c.UpdateVaultSecret(runtimeHash, name, value)
		if err != nil {
			return errMsg{err}
		}
		return secretCreatedMsg{} // reuse: triggers secrets reload
	}
}

func deleteSecretCmd(c *Client, runtimeHash, name string) tea.Cmd {
	return func() tea.Msg {
		if err := c.DeleteVaultSecret(runtimeHash, name); err != nil {
			return errMsg{err}
		}
		return secretDeletedMsg{}
	}
}

func loadVaultsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		vaults, err := c.ListVaults()
		if err != nil {
			return errMsg{err}
		}
		// Enrich vaults with agent info; failure is non-fatal — agent fields will be empty.
		agents, err := c.ListAgents()
		if err != nil {
			log.Printf("[tui] loadVaultsCmd: ListAgents: %v", err)
		}
		agentByHash := map[string]map[string]any{}
		for _, a := range agents {
			if rh := str(a, "vault_runtime_hash"); rh != "" {
				agentByHash[rh] = a
			}
		}
		for i, v := range vaults {
			if a, ok := agentByHash[str(v, "vault_runtime_hash")]; ok {
				vaults[i]["health"] = str(a, "health")
				vaults[i]["secrets_count"] = str(a, "secrets_count")
				vaults[i]["ip"] = str(a, "ip")
				vaults[i]["last_seen_ago"] = str(a, "last_seen_ago")
			}
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
		m.applySecretFilter()
		m.secretsCursor = clampCursor(m.secretsCursor, len(m.filteredSecrets))
		return m, nil

	case secretCreatedMsg:
		m.creatingSecret = false
		m.secretsLoading = true
		return m, loadSecretsCmd(c, str(m.detailVault, "vault_runtime_hash"))

	case secretDeletedMsg:
		m.showSecretDetail = false
		m.revealValue = ""
		m.secretsLoading = true
		return m, loadSecretsCmd(c, str(m.detailVault, "vault_runtime_hash"))

	case agentsLoadedMsg:
		m.agents = msg.agents
		m.loading = false
		m.agentCursor = clampCursor(m.agentCursor, len(m.agents))
		return m, nil

	case catalogLoadedMsg:
		m.catalog = msg.catalog
		m.loading = false
		m.applyCatalogFilter()
		m.catalogCursor = clampCursor(m.catalogCursor, len(m.filteredCatalog))
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
		m.secretsLoading = false
		m.metaLoading = false
		m.revealing = false
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			if !m.showDetail && !m.showSecretDetail && !m.creatingSecret && !m.searching && !m.catalogSearching && !m.confirmDelete {
				// Sub-tab header(1) + blank(1) + col header(1) = list starts at Y=4 (relative to page, +1 for main tab bar)
				switch m.tab {
				case vaultTabList:
					idx := msg.Y - 4
					if idx >= 0 && idx < len(m.vaults) {
						m.cursor = idx
					}
				case vaultTabAgents:
					idx := msg.Y - 4
					if idx >= 0 && idx < len(m.agents) {
						m.agentCursor = idx
					}
				case vaultTabCatalog:
					offset := 4
					if m.catalogQuery != "" {
						offset += 2 // search line + blank
					}
					idx := msg.Y - offset
					if idx >= 0 && idx < len(m.filteredCatalog) {
						m.catalogCursor = idx
					}
				}
			} else if m.showDetail && !m.showSecretDetail {
				// Secrets list: header(1) + blank(1) + col header(1) = starts at Y=4
				offset := 4
				if m.searchQuery != "" {
					offset += 2
				}
				idx := msg.Y - offset
				if idx >= 0 && idx < len(m.filteredSecrets) {
					m.secretsCursor = idx
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		// Delete confirm mode
		if m.confirmDelete {
			return m.updateDeleteConfirm(msg, c)
		}
		// Search mode in secrets
		if m.searching {
			return m.updateSearch(msg)
		}
		// Search mode in catalog
		if m.catalogSearching {
			return m.updateCatalogSearch(msg)
		}
		// Create secret mode
		if m.creatingSecret {
			return m.updateCreateSecret(msg, c)
		}

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
			case "d":
				m.confirmDelete = true
				m.deleteTargetName = str(m.secretDetail, "name")
			case "e":
				m.editingSecret = true
				m.editSecretName = str(m.secretDetail, "name")
				m.creatingSecret = true
				m.createName.SetValue(m.editSecretName)
				m.createValue.SetValue("")
				m.createFocus = 1
				m.createName.Blur()
				m.createValue.Focus()
				m.showSecretDetail = false
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
			return m.updateCatalog(msg, c)
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
		if m.secretsCursor < len(m.filteredSecrets)-1 {
			m.secretsCursor++
		}
	case "k", "up":
		if m.secretsCursor > 0 {
			m.secretsCursor--
		}
	case "/":
		m.searching = true
		m.searchInput.SetValue(m.searchQuery)
		m.searchInput.Focus()
		return m, nil
	case "n":
		m.creatingSecret = true
		m.createName.SetValue("")
		m.createValue.SetValue("")
		m.createFocus = 0
		m.createName.Focus()
		m.createValue.Blur()
		return m, nil
	case "enter":
		if len(m.filteredSecrets) > 0 {
			s := m.filteredSecrets[m.secretsCursor]
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

func (m vaultsModel) updateCatalog(msg tea.KeyMsg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.catalogCursor < len(m.filteredCatalog)-1 {
			m.catalogCursor++
		}
	case "k", "up":
		if m.catalogCursor > 0 {
			m.catalogCursor--
		}
	case "/":
		m.catalogSearching = true
		m.catalogSearch.SetValue(m.catalogQuery)
		m.catalogSearch.Focus()
	case "r":
		m.loading = true
		return m, loadCatalogCmd(c)
	}
	return m, nil
}

func (m vaultsModel) updateDeleteConfirm(msg tea.KeyMsg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.confirmDelete = false
		rh := str(m.detailVault, "vault_runtime_hash")
		m.showSecretDetail = false
		m.revealValue = ""
		return m, deleteSecretCmd(c, rh, m.deleteTargetName)
	case "n", "esc":
		m.confirmDelete = false
	}
	return m, nil
}

func (m vaultsModel) viewDeleteConfirm() string {
	var b strings.Builder
	b.WriteString(styleError.Render("  ⚠ " + T("vaults.delete_confirm")))
	b.WriteString("\n\n")
	b.WriteString("  " + styleValue.Render(m.deleteTargetName))
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("  y confirm  n cancel"))
	return b.String()
}

func (m vaultsModel) view(width int) string {
	if m.confirmDelete {
		return m.viewDeleteConfirm()
	}
	if m.showSecretDetail {
		return m.viewSecretDetail()
	}
	if m.creatingSecret {
		return m.viewSecretCreate()
	}
	if m.searching {
		return m.viewSearching()
	}
	if m.showDetail {
		return m.viewSecrets(width)
	}
	if m.catalogSearching {
		return m.viewCatalogSearching()
	}

	// Sub-tabs
	tabs := []struct {
		name string
		t    vaultTab
	}{
		{T("vaults.title"), vaultTabList},
		{T("vaults.agents"), vaultTabAgents},
		{T("vaults.catalog"), vaultTabCatalog},
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
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if m.offline {
		b.WriteString(styleError.Render("  ⚠ " + T("common.offline")))
		return b.String()
	}
	if len(m.vaults) == 0 {
		b.WriteString(styleDim.Render("  " + T("vaults.empty")))
		return b.String()
	}

	h := fmt.Sprintf("  %-20s %-10s %-10s %-18s %-8s %-10s", "Name", "Status", "Health", "IP", "Secrets", "Last Seen")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, v := range m.vaults {
		// Prefer display_name (human-readable label) over vault_name (internal identifier).
		// Fallback order was intentionally changed from vault_name->display_name to
		// display_name->vault_name when the enriched vault list view was introduced,
		// so that the human-friendly label is shown as the primary column.
		name := str(v, "display_name")
		if name == "" {
			name = str(v, "vault_name")
		}
		line := fmt.Sprintf("  %-20s %-10s %-10s %-18s %-8s %-10s",
			truncate(name, 18),
			str(v, "status"),
			str(v, "health"),
			truncate(str(v, "ip"), 16),
			str(v, "secrets_count"),
			str(v, "last_seen_ago"),
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
	name := str(m.detailVault, "display_name")
	if name == "" {
		name = str(m.detailVault, "vault_name")
	}
	if name == "" {
		name = str(m.detailVault, "vault_hash")
	}
	b.WriteString(styleHeader.Render(fmt.Sprintf("  %s — Secrets", name)))
	b.WriteString("\n\n")

	if m.secretsLoading {
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if m.searchQuery != "" {
		b.WriteString("  " + styleDim.Render(T("common.search")+" ") + styleReveal.Render(m.searchQuery) + "\n\n")
	}

	if len(m.filteredSecrets) == 0 {
		if m.searchQuery != "" {
			b.WriteString(styleDim.Render("  " + T("vaults.no_matches")))
		} else {
			b.WriteString(styleDim.Render("  " + T("vaults.no_secrets")))
		}
		b.WriteString("\n\n" + styleDim.Render("  / search  n create  esc back"))
		return b.String()
	}

	h := fmt.Sprintf("  %-28s %-28s %-10s %-10s", "Name", "Ref", "Scope", "Status")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, s := range m.filteredSecrets {
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
	b.WriteString(styleDim.Render("  j/k move  enter detail  / search  n create  r refresh  esc back"))
	return b.String()
}

func (m vaultsModel) viewSecretCreate() string {
	var b strings.Builder
	vaultName := str(m.detailVault, "display_name")
	if vaultName == "" {
		vaultName = str(m.detailVault, "vault_name")
	}
	if m.editingSecret {
		b.WriteString(styleHeader.Render(fmt.Sprintf("  %s — Edit: %s", vaultName, m.editSecretName)))
	} else {
		b.WriteString(styleHeader.Render(fmt.Sprintf("  %s — New Secret", vaultName)))
	}
	b.WriteString("\n\n")
	if !m.editingSecret {
		b.WriteString("  " + styleLabel.Render("Name") + "\n")
		b.WriteString("  " + m.createName.View() + "\n\n")
	}
	b.WriteString("  " + styleLabel.Render("Value") + "\n")
	b.WriteString("  " + m.createValue.View() + "\n\n")
	if m.editingSecret {
		b.WriteString(styleDim.Render("  enter update  esc cancel"))
	} else {
		b.WriteString(styleDim.Render("  tab switch  enter create  esc cancel"))
	}
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
		b.WriteString("\n  " + styleDim.Render(T("common.loading_meta")))
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
				fmt.Fprintf(&b, "    %s → %s (%s)\n",
					str(bind, "binding_type"),
					str(bind, "target_name"),
					str(bind, "binding_id"),
				)
			}
		}
	}

	// Reveal — VK: masked, VE: shown as-is
	b.WriteString("\n")
	isVK := strings.HasPrefix(ref, "VK:")
	if isVK {
		if m.revealing {
			b.WriteString("  " + styleDim.Render(T("common.decrypting")))
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
		b.WriteString(styleDim.Render("  r reveal  h hide  e edit  d delete  esc back"))
	} else {
		b.WriteString(styleDim.Render("  e edit  d delete  esc back"))
	}
	return b.String()
}

func (m vaultsModel) viewAgents(width int) string {
	var b strings.Builder

	if m.loading {
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if len(m.agents) == 0 {
		b.WriteString(styleDim.Render("  " + T("vaults.no_agents")))
		return b.String()
	}

	h := fmt.Sprintf("  %-16s %-10s %-10s %-18s %-8s %-6s %-10s", "Name", "Status", "Health", "IP", "Secrets", "Ver", "Last Seen")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, a := range m.agents {
		vname := str(a, "label")
		if vname == "" {
			vname = str(a, "vault_name")
		}
		if vname == "" {
			vname = truncate(str(a, "vault_hash"), 14)
		}
		ip := str(a, "ip")
		if port := str(a, "port"); port != "" && port != "0" {
			ip += ":" + port
		}
		line := fmt.Sprintf("  %-16s %-10s %-10s %-18s %-8s %-6s %-10s",
			truncate(vname, 14),
			str(a, "status"),
			str(a, "health"),
			truncate(ip, 16),
			str(a, "secrets_count"),
			str(a, "key_version"),
			str(a, "last_seen_ago"),
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
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if m.catalogQuery != "" {
		b.WriteString("  " + styleDim.Render(T("common.search")+" ") + styleReveal.Render(m.catalogQuery) + "\n\n")
	}

	if len(m.filteredCatalog) == 0 {
		if m.catalogQuery != "" {
			b.WriteString(styleDim.Render("  " + T("vaults.no_matches")))
		} else {
			b.WriteString(styleDim.Render("  " + T("vaults.no_catalog")))
		}
		b.WriteString("\n\n" + styleDim.Render("  / search"))
		return b.String()
	}

	h := fmt.Sprintf("  %-24s %-20s %-14s %-10s", "Name", "Ref", "Class", "Bindings")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, s := range m.filteredCatalog {
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
	b.WriteString(styleDim.Render("  j/k move  / search  tab switch"))
	return b.String()
}

// ── Search ──

func (m vaultsModel) updateSearch(msg tea.KeyMsg) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searchQuery = m.searchInput.Value()
		m.searching = false
		m.applySecretFilter()
		m.secretsCursor = 0
	case "esc":
		m.searching = false
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m vaultsModel) updateCatalogSearch(msg tea.KeyMsg) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.catalogQuery = m.catalogSearch.Value()
		m.catalogSearching = false
		m.applyCatalogFilter()
		m.catalogCursor = 0
	case "esc":
		m.catalogSearching = false
	default:
		var cmd tea.Cmd
		m.catalogSearch, cmd = m.catalogSearch.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *vaultsModel) applySecretFilter() {
	if m.searchQuery == "" {
		m.filteredSecrets = m.secrets
		return
	}
	q := strings.ToLower(m.searchQuery)
	var filtered []map[string]any
	for _, s := range m.secrets {
		name := strings.ToLower(str(s, "name"))
		ref := strings.ToLower(str(s, "token"))
		if ref == "" {
			ref = strings.ToLower(str(s, "ref"))
		}
		scope := strings.ToLower(str(s, "scope"))
		if strings.Contains(name, q) || strings.Contains(ref, q) || strings.Contains(scope, q) {
			filtered = append(filtered, s)
		}
	}
	m.filteredSecrets = filtered
}

func (m *vaultsModel) applyCatalogFilter() {
	if m.catalogQuery == "" {
		m.filteredCatalog = m.catalog
		return
	}
	q := strings.ToLower(m.catalogQuery)
	var filtered []map[string]any
	for _, s := range m.catalog {
		name := strings.ToLower(str(s, "secret_name"))
		ref := strings.ToLower(str(s, "ref_canonical"))
		class := strings.ToLower(str(s, "class"))
		if strings.Contains(name, q) || strings.Contains(ref, q) || strings.Contains(class, q) {
			filtered = append(filtered, s)
		}
	}
	m.filteredCatalog = filtered
}

func (m vaultsModel) viewSearching() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("  " + T("vaults.search")))
	b.WriteString("\n\n")
	b.WriteString("  " + m.searchInput.View() + "\n\n")
	b.WriteString(styleDim.Render("  enter search  esc cancel"))
	return b.String()
}

func (m vaultsModel) viewCatalogSearching() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("  " + T("vaults.search_catalog")))
	b.WriteString("\n\n")
	b.WriteString("  " + m.catalogSearch.View() + "\n\n")
	b.WriteString(styleDim.Render("  enter search  esc cancel"))
	return b.String()
}

// ── Create/Edit secret ──

func (m vaultsModel) updateCreateSecret(msg tea.KeyMsg, c *Client) (vaultsModel, tea.Cmd) {
	switch msg.String() {
	case "tab", "shift+tab":
		if m.createFocus == 0 {
			m.createFocus = 1
			m.createName.Blur()
			m.createValue.Focus()
		} else {
			m.createFocus = 0
			m.createValue.Blur()
			m.createName.Focus()
		}
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.createName.Value())
		value := strings.TrimSpace(m.createValue.Value())
		if name == "" || value == "" {
			return m, nil
		}
		runtimeHash := str(m.detailVault, "vault_runtime_hash")
		if m.editingSecret {
			m.editingSecret = false
			return m, updateSecretCmd(c, runtimeHash, m.editSecretName, value)
		}
		return m, createSecretCmd(c, runtimeHash, name, value)
	case "esc":
		m.creatingSecret = false
		m.editingSecret = false
	default:
		var cmd tea.Cmd
		if m.createFocus == 0 {
			m.createName, cmd = m.createName.Update(msg)
		} else {
			m.createValue, cmd = m.createValue.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func clampCursor(cursor, length int) int {
	if cursor >= length {
		return max(0, length-1)
	}
	return cursor
}
