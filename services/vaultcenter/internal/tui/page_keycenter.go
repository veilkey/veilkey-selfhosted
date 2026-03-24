package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type kcView int

const (
	kcList kcView = iota
	kcDetail
	kcCreate
	kcPromote
)

type keycenterModel struct {
	refs      []TempRef
	cursor    int
	loading   bool
	offline   bool
	subview   kcView
	creating  bool

	// Detail
	detailRef TempRef
	revealed  string
	revealing bool

	// Create
	nameInput  textinput.Model
	valueInput textinput.Model
	focusIdx   int

	// Promote
	vaults      []map[string]any
	vaultCursor int
	promoting   bool
}

func newKeycenterModel() keycenterModel {
	ni := textinput.New()
	ni.Placeholder = "secret name (optional)"
	ni.CharLimit = 128
	ni.Width = 50

	vi := textinput.New()
	vi.Placeholder = "plaintext value"
	vi.CharLimit = 4096
	vi.Width = 50
	vi.EchoMode = textinput.EchoPassword
	vi.EchoCharacter = '•'

	return keycenterModel{loading: true, nameInput: ni, valueInput: vi}
}

func (m keycenterModel) update(msg tea.Msg, c *Client) (keycenterModel, tea.Cmd) {
	switch msg := msg.(type) {
	case refsLoadedMsg:
		m.refs = msg.refs
		m.loading = false
		m.offline = false
		if m.cursor >= len(m.refs) {
			m.cursor = max(0, len(m.refs)-1)
		}
		return m, nil

	case errMsg:
		m.loading = false
		m.offline = true
		return m, nil

	case refRevealedMsg:
		m.revealed = msg.value
		m.revealing = false
		return m, nil

	case refCreatedMsg:
		m.subview = kcList
		m.creating = false
		return m, loadRefsCmd(c)

	case refDeletedMsg:
		m.subview = kcList
		return m, loadRefsCmd(c)

	case vaultsLoadedMsg:
		m.vaults = msg.vaults
		m.vaultCursor = 0
		return m, nil

	case refPromotedMsg:
		m.subview = kcList
		m.promoting = false
		return m, loadRefsCmd(c)

	case tea.KeyMsg:
		switch m.subview {
		case kcPromote:
			return m.updatePromote(msg, c)
		case kcList:
			return m.updateList(msg, c)
		case kcDetail:
			return m.updateDetail(msg, c)
		case kcCreate:
			return m.updateCreate(msg, c)
		}
	}

	// Update text inputs if creating
	if m.subview == kcCreate {
		var cmd tea.Cmd
		if m.focusIdx == 0 {
			m.nameInput, cmd = m.nameInput.Update(msg)
		} else {
			m.valueInput, cmd = m.valueInput.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m keycenterModel) updateList(msg tea.KeyMsg, c *Client) (keycenterModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.refs)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.refs) > 0 {
			m.detailRef = m.refs[m.cursor]
			m.revealed = ""
			m.revealing = false
			m.subview = kcDetail
		}
	case "n":
		m.subview = kcCreate
		m.creating = true
		m.nameInput.SetValue("")
		m.valueInput.SetValue("")
		m.focusIdx = 0
		m.nameInput.Focus()
		m.valueInput.Blur()
	case "d":
		if len(m.refs) > 0 {
			return m, func() tea.Msg {
				return errMsg{fmt.Errorf("temp refs expire automatically — manual delete not supported")}
			}
		}
	case "r":
		m.loading = true
		return m, loadRefsCmd(c)
	}
	return m, nil
}

func (m keycenterModel) updateDetail(msg tea.KeyMsg, c *Client) (keycenterModel, tea.Cmd) {
	switch msg.String() {
	case "r":
		if !m.revealing && m.revealed == "" {
			m.revealing = true
			return m, revealRefCmd(c, m.detailRef.RefCanonical)
		}
	case "h":
		m.revealed = ""
	case "p":
		m.subview = kcPromote
		m.vaultCursor = 0
		return m, loadVaultsCmd(c)
	case "esc":
		m.subview = kcList
	}
	return m, nil
}

func (m keycenterModel) updatePromote(msg tea.KeyMsg, c *Client) (keycenterModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.vaultCursor < len(m.vaults)-1 {
			m.vaultCursor++
		}
	case "k", "up":
		if m.vaultCursor > 0 {
			m.vaultCursor--
		}
	case "enter":
		if len(m.vaults) > 0 {
			v := m.vaults[m.vaultCursor]
			m.promoting = true
			return m, promoteRefCmd(c, m.detailRef.RefCanonical, m.detailRef.SecretName, str(v, "vault_hash"))
		}
	case "esc":
		m.subview = kcDetail
	}
	return m, nil
}

func promoteRefCmd(c *Client, ref, name, vaultHash string) tea.Cmd {
	return func() tea.Msg {
		_, err := c.PromoteRef(ref, name, vaultHash)
		if err != nil {
			return errMsg{err}
		}
		return refPromotedMsg{}
	}
}

func (m keycenterModel) updateCreate(msg tea.KeyMsg, c *Client) (keycenterModel, tea.Cmd) {
	switch msg.String() {
	case "tab", "shift+tab":
		if m.focusIdx == 0 {
			m.focusIdx = 1
			m.nameInput.Blur()
			m.valueInput.Focus()
		} else {
			m.focusIdx = 0
			m.valueInput.Blur()
			m.nameInput.Focus()
		}
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.valueInput.Value())
		if value == "" {
			return m, nil
		}
		name := strings.TrimSpace(m.nameInput.Value())
		return m, createRefCmd(c, name, value)
	case "esc":
		m.subview = kcList
		m.creating = false
	default:
		var cmd tea.Cmd
		if m.focusIdx == 0 {
			m.nameInput, cmd = m.nameInput.Update(msg)
		} else {
			m.valueInput, cmd = m.valueInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m keycenterModel) view(width int) string {
	switch m.subview {
	case kcDetail:
		return m.viewDetail()
	case kcCreate:
		return m.viewCreate()
	case kcPromote:
		return m.viewPromote(width)
	default:
		return m.viewList(width)
	}
}

func (m keycenterModel) viewList(width int) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("  Temp Refs"))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(styleDim.Render("  Loading..."))
		return b.String()
	}
	if m.offline {
		b.WriteString(styleError.Render("  ⚠ Cannot reach VaultCenter"))
		b.WriteString("\n\n")
		b.WriteString(styleDim.Render("  r retry  q quit"))
		return b.String()
	}
	if len(m.refs) == 0 {
		b.WriteString(styleDim.Render("  No temp refs."))
		b.WriteString("\n")
	} else {
		header := fmt.Sprintf("  %-30s %-20s %-10s %-20s", "Ref", "Name", "Status", "Created")
		b.WriteString(styleDim.Render(header))
		b.WriteString("\n")
		for i, ref := range m.refs {
			name := ref.SecretName
			if name == "" {
				name = "-"
			}
			line := fmt.Sprintf("  %-30s %-20s %-10s %-20s",
				truncate(ref.RefCanonical, 28),
				truncate(name, 18),
				ref.Status,
				ref.CreatedAt.Format("01-02 15:04"),
			)
			if i == m.cursor {
				line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  enter detail  n new  r refresh"))
	return b.String()
}

func (m keycenterModel) viewDetail() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("  Ref Detail"))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString("  ")
		b.WriteString(styleLabel.Render(label))
		b.WriteString(styleValue.Render(value))
		b.WriteString("\n")
	}
	row("Ref", m.detailRef.RefCanonical)
	row("Name", m.detailRef.SecretName)
	row("Status", m.detailRef.Status)
	row("Created", m.detailRef.CreatedAt.Format("2006-01-02 15:04:05"))
	if m.detailRef.ExpiresAt != nil {
		row("Expires", m.detailRef.ExpiresAt.Format("2006-01-02 15:04:05"))
	}
	b.WriteString("\n")
	if m.revealing {
		b.WriteString("  " + styleDim.Render("Decrypting..."))
	} else if m.revealed != "" {
		b.WriteString("  " + styleLabel.Render("Value"))
		b.WriteString(styleReveal.Render(m.revealed))
		b.WriteString("\n  " + styleDim.Render("h hide"))
	} else {
		b.WriteString("  " + styleDim.Render("r reveal"))
	}
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("  r reveal  h hide  p promote  esc back"))
	return b.String()
}

func (m keycenterModel) viewPromote(width int) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("  Promote: %s → Vault", m.detailRef.RefCanonical)))
	b.WriteString("\n\n")

	if m.promoting {
		b.WriteString("  " + styleDim.Render("Promoting..."))
		return b.String()
	}
	if len(m.vaults) == 0 {
		b.WriteString(styleDim.Render("  Loading vaults..."))
		return b.String()
	}

	b.WriteString(styleDim.Render("  Select target vault:") + "\n\n")
	for i, v := range m.vaults {
		name := str(v, "display_name")
		if name == "" {
			name = str(v, "vault_name")
		}
		if name == "" {
			name = str(v, "vault_hash")
		}
		line := fmt.Sprintf("  %-24s %s", truncate(name, 22), str(v, "status"))
		if i == m.vaultCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 60)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  enter promote  esc cancel"))
	return b.String()
}

func (m keycenterModel) viewCreate() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("  Create Temp Ref"))
	b.WriteString("\n\n")
	b.WriteString("  " + styleLabel.Render("Name") + "\n")
	b.WriteString("  " + m.nameInput.View() + "\n\n")
	b.WriteString("  " + styleLabel.Render("Value") + "\n")
	b.WriteString("  " + m.valueInput.View() + "\n\n")
	b.WriteString(styleDim.Render("  tab switch  enter submit  esc cancel"))
	return b.String()
}

