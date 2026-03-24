package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type auditModel struct {
	loading bool
	offline bool

	events []map[string]any
	cursor int
}

type auditLoadedMsg struct{ events []map[string]any }

func newAuditModel() auditModel {
	return auditModel{loading: true}
}

func loadAuditCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		events, err := c.ListAuditEvents()
		if err != nil {
			return errMsg{err}
		}
		return auditLoadedMsg{events}
	}
}

func (m auditModel) update(msg tea.Msg, c *Client) (auditModel, tea.Cmd) {
	switch msg := msg.(type) {
	case auditLoadedMsg:
		m.events = msg.events
		m.loading = false
		m.offline = false
		m.cursor = clampCursor(m.cursor, len(m.events))
		return m, nil

	case errMsg:
		m.loading = false
		m.offline = true
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			// Header(1) + blank(1) + col header(1) + main tab(1) = row 4
			idx := msg.Y - 4
			if idx >= 0 && idx < len(m.events) {
				m.cursor = idx
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.events)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "r":
			m.loading = true
			return m, loadAuditCmd(c)
		}
	}
	return m, nil
}

func (m auditModel) view(width int) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("  " + T("audit.title")))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if m.offline {
		b.WriteString(styleError.Render("  ⚠ " + T("common.offline")))
		return b.String()
	}
	if len(m.events) == 0 {
		b.WriteString(styleDim.Render("  " + T("audit.empty")))
		return b.String()
	}

	h := fmt.Sprintf("  %-18s %-14s %-20s %-20s %-14s", "Time", "Entity", "Action", "Actor", "Reason")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")

	for i, ev := range m.events {
		ts := str(ev, "created_at")
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			ts = t.Format("01-02 15:04")
		}
		entity := str(ev, "entity_type")
		if eid := str(ev, "entity_id"); eid != "" {
			entity += ":" + truncate(eid, 6)
		}
		line := fmt.Sprintf("  %-18s %-14s %-20s %-20s %-14s",
			ts,
			truncate(entity, 12),
			truncate(str(ev, "action"), 18),
			truncate(str(ev, "actor_type"), 18),
			truncate(str(ev, "reason"), 12),
		)
		if i == m.cursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  r refresh"))
	return b.String()
}
