package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type page int

const (
	pageLogin page = iota
	pageKeycenter
	pageVaults
	pageFunctions
	pageAudit
	pageSettings
)

var pageNames = []string{"Keycenter", "Vaults", "Functions", "Audit", "Settings"}
var pages = []page{pageKeycenter, pageVaults, pageFunctions, pageAudit, pageSettings}

// Model is the top-level bubbletea model.
type Model struct {
	client    *Client
	width     int
	height    int
	activePage page
	status    string
	err       error

	// Sub-models
	login     loginModel
	keycenter keycenterModel
	vaults    vaultsModel
	functions functionsModel
	audit     auditModel
	settings  settingsModel
}

// NewModel creates a new TUI model.
func NewModel(serverURL string) Model {
	client := NewClient(serverURL)
	return Model{
		client:     client,
		activePage: pageLogin,
		status:     "connecting...",
		login:      newLoginModel(),
		keycenter:  newKeycenterModel(),
		vaults:     newVaultsModel(),
		functions:  newFunctionsModel(),
		audit:      newAuditModel(),
		settings:   newSettingsModel(),
	}
}

func (m Model) Init() tea.Cmd {
	return checkStatusCmd(m.client)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.activePage == pageLogin || !m.isEditing() {
				return m, tea.Quit
			}
		case "1", "2", "3", "4", "5":
			if m.activePage != pageLogin && !m.isEditing() {
				idx := int(msg.String()[0] - '1')
				if idx < len(pages) {
					return m.switchPage(pages[idx])
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			// Tab bar click (first line, row 0)
			if msg.Y == 0 && m.activePage != pageLogin {
				clickedTab := detectTabClick(msg.X, pageNames)
				if clickedTab >= 0 && clickedTab < len(pages) {
					return m.switchPage(pages[clickedTab])
				}
			}
		}

	case statusMsg:
		m.status = msg.status
		// Fall through to let login handle it too

	case errMsg:
		m.err = msg.err

	case loginSuccessMsg:
		m.status = "ready"
		return m.switchPage(pageKeycenter)

	case loginFailMsg:
		// Let login handle it
	}

	// Delegate to active page
	var cmd tea.Cmd
	switch m.activePage {
	case pageLogin:
		m.login, cmd = m.login.update(msg, m.client)
	case pageKeycenter:
		m.keycenter, cmd = m.keycenter.update(msg, m.client)
	case pageVaults:
		m.vaults, cmd = m.vaults.update(msg, m.client)
	case pageFunctions:
		m.functions, cmd = m.functions.update(msg, m.client)
	case pageAudit:
		m.audit, cmd = m.audit.update(msg, m.client)
	case pageSettings:
		m.settings, cmd = m.settings.update(msg, m.client)
	}
	return m, cmd
}

func (m Model) View() string {
	var content string

	if m.activePage == pageLogin {
		content = m.login.view(m.width)
	} else {
		tabs := m.renderTabs()
		var pageContent string
		switch m.activePage {
		case pageKeycenter:
			pageContent = m.keycenter.view(m.width)
		case pageVaults:
			pageContent = m.vaults.view(m.width)
		case pageFunctions:
			pageContent = m.functions.view(m.width)
		case pageAudit:
			pageContent = m.audit.view(m.width)
		case pageSettings:
			pageContent = m.settings.view(m.width)
		}
		content = lipgloss.JoinVertical(lipgloss.Left, tabs, pageContent)
	}

	// Status bar
	statusText := fmt.Sprintf(" VaultCenter: %s", m.status)
	if m.err != nil {
		statusText += "  " + styleError.Render(m.err.Error())
	}
	status := styleStatusBar.Render(statusText)

	return lipgloss.JoinVertical(lipgloss.Left, content, status)
}

func (m Model) renderTabs() string {
	var tabs []string
	for i, name := range pageNames {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		if pages[i] == m.activePage {
			tabs = append(tabs, styleActive.Render(label))
		} else {
			tabs = append(tabs, styleInactive.Render(label))
		}
	}
	return "  " + strings.Join(tabs, " ") + "\n"
}

func (m Model) switchPage(p page) (Model, tea.Cmd) {
	m.activePage = p
	m.err = nil
	switch p {
	case pageKeycenter:
		m.keycenter = newKeycenterModel()
		return m, loadRefsCmd(m.client)
	case pageVaults:
		m.vaults = newVaultsModel()
		return m, loadVaultsCmd(m.client)
	case pageFunctions:
		m.functions = newFunctionsModel()
		return m, loadFunctionsCmd(m.client)
	case pageAudit:
		m.audit = newAuditModel()
		return m, loadAuditCmd(m.client)
	case pageSettings:
		m.settings = newSettingsModel()
		return m, loadSettingsCmd(m.client)
	}
	return m, nil
}

// detectTabClick returns the tab index clicked based on X position.
// Tab format: "  | 1 Name | 2 Name | ..."
func detectTabClick(x int, names []string) int {
	pos := 2 // initial margin
	for i, name := range names {
		// Each tab: " N Name " with padding
		tabWidth := len(name) + 4 // " N Name "
		if x >= pos && x < pos+tabWidth {
			return i
		}
		pos += tabWidth + 1 // +1 for space between tabs
	}
	return -1
}

func (m Model) isEditing() bool {
	if m.activePage == pageKeycenter {
		return m.keycenter.creating || m.keycenter.subview == kcPromote
	}
	if m.activePage == pageVaults {
		return m.vaults.creatingSecret || m.vaults.searching || m.vaults.catalogSearching
	}
	if m.activePage == pageSettings {
		return m.settings.creatingToken
	}
	return false
}
