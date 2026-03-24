package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fnTab int

const (
	fnTabList fnTab = iota
	fnTabBindings
)

type functionsModel struct {
	tab     fnTab
	loading bool
	offline bool

	// List
	functions []map[string]any
	cursor    int

	// Detail
	showDetail bool
	detail     map[string]any
	runOutput  string
	running    bool

	// Bindings
	bindings      []map[string]any
	bindingCursor int
}

type functionsLoadedMsg struct{ functions []map[string]any }
type bindingsLoadedMsg struct{ bindings []map[string]any }
type functionRunMsg struct{ output string }

func newFunctionsModel() functionsModel {
	return functionsModel{loading: true}
}

func loadFunctionsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		fns, err := c.ListFunctions()
		if err != nil {
			return errMsg{err}
		}
		return functionsLoadedMsg{fns}
	}
}

func loadBindingsCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		bindings, err := c.ListBindings()
		if err != nil {
			return errMsg{err}
		}
		return bindingsLoadedMsg{bindings}
	}
}

func runFunctionCmd(c *Client, name string) tea.Cmd {
	return func() tea.Msg {
		data, err := c.RunFunction(name)
		if err != nil {
			return errMsg{err}
		}
		b, _ := jsonMarshal(data)
		return functionRunMsg{string(b)}
	}
}

func (m functionsModel) update(msg tea.Msg, c *Client) (functionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case functionsLoadedMsg:
		m.functions = msg.functions
		m.loading = false
		m.offline = false
		m.cursor = clampCursor(m.cursor, len(m.functions))
		return m, nil

	case bindingsLoadedMsg:
		m.bindings = msg.bindings
		m.loading = false
		m.bindingCursor = clampCursor(m.bindingCursor, len(m.bindings))
		return m, nil

	case functionRunMsg:
		m.runOutput = msg.output
		m.running = false
		return m, nil

	case errMsg:
		m.loading = false
		m.offline = true
		return m, nil

	case tea.KeyMsg:
		if !m.showDetail {
			switch msg.String() {
			case "tab":
				if m.tab == fnTabList {
					m.tab = fnTabBindings
					m.loading = true
					return m, loadBindingsCmd(c)
				}
				m.tab = fnTabList
				return m, nil
			}
		}

		if m.showDetail {
			return m.updateDetail(msg, c)
		}
		switch m.tab {
		case fnTabList:
			return m.updateList(msg, c)
		case fnTabBindings:
			return m.updateBindings(msg, c)
		}
	}
	return m, nil
}

func (m functionsModel) updateList(msg tea.KeyMsg, c *Client) (functionsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.functions)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.functions) > 0 {
			m.showDetail = true
			m.detail = m.functions[m.cursor]
			m.runOutput = ""
			m.running = false
		}
	case "r":
		m.loading = true
		return m, loadFunctionsCmd(c)
	}
	return m, nil
}

func (m functionsModel) updateDetail(msg tea.KeyMsg, c *Client) (functionsModel, tea.Cmd) {
	switch msg.String() {
	case "x":
		if !m.running {
			m.running = true
			m.runOutput = ""
			return m, runFunctionCmd(c, str(m.detail, "name"))
		}
	case "esc":
		m.showDetail = false
	}
	return m, nil
}

func (m functionsModel) updateBindings(msg tea.KeyMsg, c *Client) (functionsModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.bindingCursor < len(m.bindings)-1 {
			m.bindingCursor++
		}
	case "k", "up":
		if m.bindingCursor > 0 {
			m.bindingCursor--
		}
	case "r":
		m.loading = true
		return m, loadBindingsCmd(c)
	}
	return m, nil
}

func (m functionsModel) view(width int) string {
	if m.showDetail {
		return m.viewDetail()
	}

	tabs := []struct {
		name string
		t    fnTab
	}{
		{T("fn.title"), fnTabList},
		{T("fn.bindings"), fnTabBindings},
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
	case fnTabBindings:
		return header + m.viewBindings(width)
	default:
		return header + m.viewList(width)
	}
}

func (m functionsModel) viewList(width int) string {
	var b strings.Builder

	if m.loading {
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if m.offline {
		b.WriteString(styleError.Render("  ⚠ " + T("common.offline")))
		return b.String()
	}
	if len(m.functions) == 0 {
		b.WriteString(styleDim.Render("  " + T("fn.empty")))
		return b.String()
	}

	h := fmt.Sprintf("  %-24s %-30s %-16s", "Name", "Command", "Category")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, fn := range m.functions {
		line := fmt.Sprintf("  %-24s %-30s %-16s",
			truncate(str(fn, "name"), 22),
			truncate(str(fn, "command"), 28),
			truncate(str(fn, "category"), 14),
		)
		if i == m.cursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  enter detail  tab switch  r refresh"))
	return b.String()
}

func (m functionsModel) viewDetail() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("  Function: %s", str(m.detail, "name"))))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString("  ")
		b.WriteString(styleLabel.Render(label))
		b.WriteString(styleValue.Render(value))
		b.WriteString("\n")
	}
	row("Name", str(m.detail, "name"))
	row("Command", str(m.detail, "command"))
	row("Category", str(m.detail, "category"))

	b.WriteString("\n")
	if m.running {
		b.WriteString("  " + styleDim.Render(T("common.running")))
	} else if m.runOutput != "" {
		b.WriteString("  " + styleLabel.Render("Output") + "\n")
		b.WriteString("  " + styleValue.Render(truncate(m.runOutput, 300)))
	}
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("  x run  esc back"))
	return b.String()
}

func (m functionsModel) viewBindings(width int) string {
	var b strings.Builder

	if m.loading {
		b.WriteString(styleDim.Render("  " + T("common.loading")))
		return b.String()
	}
	if m.offline {
		b.WriteString(styleError.Render("  ⚠ " + T("common.offline")))
		return b.String()
	}
	if len(m.bindings) == 0 {
		b.WriteString(styleDim.Render("  " + T("fn.no_bindings")))
		return b.String()
	}

	h := fmt.Sprintf("  %-24s %-20s %-14s %-10s", "Secret Name", "Ref", "Class", "Bindings")
	b.WriteString(styleDim.Render(h))
	b.WriteString("\n")
	for i, bind := range m.bindings {
		line := fmt.Sprintf("  %-24s %-20s %-14s %-10s",
			truncate(str(bind, "secret_name"), 22),
			truncate(str(bind, "ref_canonical"), 18),
			str(bind, "class"),
			str(bind, "binding_count"),
		)
		if i == m.bindingCursor {
			line = lipgloss.NewStyle().Background(colorHighlight).Foreground(colorFg).Width(max(width-4, 80)).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  j/k move  tab switch"))
	return b.String()
}
