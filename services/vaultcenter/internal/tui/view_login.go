package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type loginStep int

const (
	loginStepCheckStatus loginStep = iota
	loginStepUnlock
	loginStepAuth
)

type authMethod int

const (
	authTOTP authMethod = iota
	authPassword
)

type loginModel struct {
	step         loginStep
	method       authMethod
	kekInput     textinput.Model
	totpInput    textinput.Model
	pwInput      textinput.Model
	logging      bool
	errText      string
	serverLocked bool
}

type unlockSuccessMsg struct{}
type unlockFailMsg struct{ err string }

func newLoginModel() loginModel {
	kek := textinput.New()
	kek.Placeholder = "master password (KEK)"
	kek.EchoMode = textinput.EchoPassword
	kek.EchoCharacter = '•'
	kek.Width = 40

	totp := textinput.New()
	totp.Placeholder = "6-digit TOTP code"
	totp.CharLimit = 6
	totp.Width = 20

	pw := textinput.New()
	pw.Placeholder = "admin password"
	pw.EchoMode = textinput.EchoPassword
	pw.EchoCharacter = '•'
	pw.Width = 40

	return loginModel{
		step:     loginStepCheckStatus,
		method:   authTOTP,
		kekInput: kek,
		totpInput: totp,
		pwInput:  pw,
	}
}

func unlockCmd(c *Client, password string) tea.Cmd {
	return func() tea.Msg {
		if err := c.Unlock(password); err != nil {
			return unlockFailMsg{err.Error()}
		}
		return unlockSuccessMsg{}
	}
}

func loginTOTPCmd(c *Client, code string) tea.Cmd {
	return func() tea.Msg {
		if err := c.LoginTOTP(code); err != nil {
			return loginFailMsg{err.Error()}
		}
		return loginSuccessMsg{}
	}
}

func loginPasswordCmd(c *Client, password string) tea.Cmd {
	return func() tea.Msg {
		if err := c.LoginPassword(password); err != nil {
			return loginFailMsg{err.Error()}
		}
		return loginSuccessMsg{}
	}
}

func (m loginModel) update(msg tea.Msg, c *Client) (loginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case statusMsg:
		switch msg.status {
		case "locked":
			m.step = loginStepUnlock
			m.serverLocked = true
			m.kekInput.Focus()
		case "ready":
			m.step = loginStepAuth
			m.focusActiveInput()
		default:
			m.errText = "Server is " + msg.status
		}
		return m, nil

	case unlockSuccessMsg:
		m.step = loginStepAuth
		m.logging = false
		m.errText = ""
		m.focusActiveInput()
		return m, nil

	case unlockFailMsg:
		m.logging = false
		m.errText = msg.err
		m.kekInput.SetValue("")
		m.kekInput.Focus()
		return m, nil

	case loginFailMsg:
		m.logging = false
		m.errText = msg.err
		if m.method == authTOTP {
			m.totpInput.SetValue("")
			m.totpInput.Focus()
		} else {
			m.pwInput.SetValue("")
			m.pwInput.Focus()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m.handleSubmit(c)
		case "tab":
			if m.step == loginStepAuth {
				if m.method == authTOTP {
					m.method = authPassword
				} else {
					m.method = authTOTP
				}
				m.errText = ""
				m.focusActiveInput()
				return m, nil
			}
		}
	}

	// Update active input
	var cmd tea.Cmd
	switch m.step {
	case loginStepUnlock:
		m.kekInput, cmd = m.kekInput.Update(msg)
	case loginStepAuth:
		if m.method == authTOTP {
			m.totpInput, cmd = m.totpInput.Update(msg)
		} else {
			m.pwInput, cmd = m.pwInput.Update(msg)
		}
	}
	return m, cmd
}

func (m *loginModel) focusActiveInput() {
	m.totpInput.Blur()
	m.pwInput.Blur()
	if m.method == authTOTP {
		m.totpInput.SetValue("")
		m.totpInput.Focus()
	} else {
		m.pwInput.SetValue("")
		m.pwInput.Focus()
	}
}

func (m loginModel) handleSubmit(c *Client) (loginModel, tea.Cmd) {
	switch m.step {
	case loginStepUnlock:
		pw := strings.TrimSpace(m.kekInput.Value())
		if pw == "" {
			return m, nil
		}
		m.logging = true
		m.errText = ""
		return m, unlockCmd(c, pw)
	case loginStepAuth:
		if m.method == authTOTP {
			code := strings.TrimSpace(m.totpInput.Value())
			if code == "" {
				return m, nil
			}
			m.logging = true
			m.errText = ""
			return m, loginTOTPCmd(c, code)
		} else {
			pw := strings.TrimSpace(m.pwInput.Value())
			if pw == "" {
				return m, nil
			}
			m.logging = true
			m.errText = ""
			return m, loginPasswordCmd(c, pw)
		}
	}
	return m, nil
}

func (m loginModel) view(width int) string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("🔐 VeilKey VaultCenter"))
	b.WriteString("\n\n")

	if m.step == loginStepCheckStatus {
		b.WriteString("  " + styleDim.Render("Connecting..."))
		return b.String()
	}

	if m.logging {
		if m.step == loginStepUnlock {
			b.WriteString("  " + styleDim.Render("Unlocking server..."))
		} else {
			b.WriteString("  " + styleDim.Render("Authenticating..."))
		}
		return b.String()
	}

	switch m.step {
	case loginStepUnlock:
		b.WriteString("  " + styleError.Render("Server is locked") + "\n\n")
		b.WriteString("  " + styleLabel.Render("Master Key") + "\n")
		b.WriteString("  " + m.kekInput.View() + "\n")
	case loginStepAuth:
		if m.serverLocked {
			b.WriteString("  " + styleSuccess.Render("✓ Server unlocked") + "\n\n")
		}

		// Auth method tabs
		totpTab := styleInactive.Render(" TOTP ")
		pwTab := styleInactive.Render(" Password ")
		if m.method == authTOTP {
			totpTab = styleActive.Render(" TOTP ")
		} else {
			pwTab = styleActive.Render(" Password ")
		}
		b.WriteString("  " + totpTab + " " + pwTab + "\n\n")

		if m.method == authTOTP {
			b.WriteString("  " + styleLabel.Render("TOTP Code") + "\n")
			b.WriteString("  " + m.totpInput.View() + "\n")
		} else {
			b.WriteString("  " + styleLabel.Render("Admin Password") + "\n")
			b.WriteString("  " + m.pwInput.View() + "\n")
		}
	}

	if m.errText != "" {
		b.WriteString("\n  " + styleError.Render(m.errText))
	}

	b.WriteString("\n\n")
	if m.step == loginStepAuth {
		b.WriteString(styleDim.Render("  enter submit  tab switch method  q quit"))
	} else {
		b.WriteString(styleDim.Render("  enter submit  q quit"))
	}

	return b.String()
}
