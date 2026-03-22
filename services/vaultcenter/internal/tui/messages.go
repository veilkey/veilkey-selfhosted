package tui

import tea "github.com/charmbracelet/bubbletea"

// Common messages
type errMsg struct{ err error }
type statusMsg struct{ status string }

// Keycenter messages
type refsLoadedMsg struct{ refs []TempRef }
type refRevealedMsg struct{ value string }
type refCreatedMsg struct{ ref string }
type refDeletedMsg struct{}
type refPromotedMsg struct{}

// Auth messages
type loginSuccessMsg struct{}
type loginFailMsg struct{ err string }

// Commands

func loadRefsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		refs, err := c.ListTempRefs()
		if err != nil {
			return errMsg{err}
		}
		return refsLoadedMsg{refs}
	}
}

func revealRefCmd(c *Client, ref string) tea.Cmd {
	return func() tea.Msg {
		val, err := c.RevealRef(ref)
		if err != nil {
			return errMsg{err}
		}
		return refRevealedMsg{val}
	}
}

func createRefCmd(c *Client, name, value string) tea.Cmd {
	return func() tea.Msg {
		result, err := c.CreateTempRef(name, value)
		if err != nil {
			return errMsg{err}
		}
		ref, _ := result["ref"].(string)
		return refCreatedMsg{ref}
	}
}

func checkStatusCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		data, err := c.Status()
		if err != nil {
			return statusMsg{"offline"}
		}
		if locked, ok := data["locked"].(bool); ok && locked {
			return statusMsg{"locked"}
		}
		return statusMsg{"ready"}
	}
}
