package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type auditTab int

const (
	auditTabRecent auditTab = iota
	auditTabCatalog
)

type auditModel struct {
	tab     auditTab
	loading bool
	offline bool

	events      []map[string]any
	cursor      int

	catalogEvents []map[string]any
	catalogCursor int
}

type auditLoadedMsg struct{ events []map[string]any }
type catalogAuditMsg struct{ events []map[string]any }

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

func loadCatalogAuditCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		events, err := c.ListCatalogAudit()
		if err != nil {
			return errMsg{err}
		}
		return catalogAuditMsg{events}
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

	case catalogAuditMsg:
		m.catalogEvents = msg.events
		m.loading = false
		m.catalogCursor = clampCursor(m.catalogCursor, len(m.catalogEvents))
		return m, nil

	case errMsg:
		m.loading = false
		m.offline = true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.tab == auditTabRecent {
				m.tab = auditTabCatalog
				m.loading = true
				return m, loadCatalogAuditCmd(c)
			}
			m.tab = auditTabRecent
			return m, nil
		case "j", "down":
			if m.tab == auditTabRecent && m.cursor < len(m.events)-1 {
				m.cursor++
			}
			if m.tab == auditTabCatalog && m.catalogCursor < len(m.catalogEvents)-1 {
				m.catalogCursor++
			}
		case "k", "up":
			if m.tab == auditTabRecent && m.cursor > 0 {
				m.cursor--
			}
			if m.tab == auditTabCatalog && m.catalogCursor > 0 {
				m.catalogCursor--
			}
		case "r":
			m.loading = true
			if m.tab == auditTabRecent {
				return m, loadAuditCmd(c)
			}
			return m, loadCatalogAuditCmd(c)
		}
	}
	return m, nil
}

func (m auditModel) view(width int) string {
	tabs := []struct {
		name string
		t    auditTab
	}{
		{"Recent", auditTabRecent},
		{"Catalog", auditTabCatalog},
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
	case auditTabCatalog:
		return header + renderAuditTable(m.catalogEvents, m.catalogCursor, m.loading, m.offline, width)
	default:
		return header + renderAuditTable(m.events, m.cursor, m.loading, m.offline, width)
	}
}

func renderAuditTable(events []map[string]any, cursor int, loading, offline bool, width int) string {
	var b strings.Builder

	if loading {
		b.WriteString(styleDim.Render("  Loading..."))
		return b.String()
	}
	if offline {
		b.WriteString(styleError.Render("  ⚠ Cannot reach VaultCenter"))
		return b.String()
	}
	if len(events) == 0 {
		b.WriteString(styleDim.Render("  No audit events."))
		return b.String()
	}

	h := fmt.Sprintf("  %-18s %-14s %-20s %-20s %-14s", "Time", "Entity", "Action", "Actor", "Reason")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")

	for i, ev := range events {
		ts := str(ev, "created_at")
		if len(ts) > 16 {
			ts = ts[5:16] // MM-DD HH:MM
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
		if i == cursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  tab switch  r refresh"))
	return b.String()
}
