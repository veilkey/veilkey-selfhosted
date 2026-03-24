package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Domain: truncate must never produce broken UTF-8
func TestTruncateASCII(t *testing.T) {
	if r := truncate("hello world", 5); r != "hel.." {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateExact(t *testing.T) {
	if r := truncate("hello", 5); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateShorter(t *testing.T) {
	if r := truncate("hi", 5); r != "hi" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateUnicode(t *testing.T) {
	// Korean: 3 bytes per char, must not break mid-char
	r := truncate("비밀번호입니다", 5)
	if len([]rune(r)) != 5 {
		t.Fatalf("expected 5 runes, got %d: %q", len([]rune(r)), r)
	}
	// Must be valid UTF-8
	for i, c := range r {
		if c == 0xFFFD {
			t.Fatalf("broken UTF-8 at position %d", i)
		}
	}
}

func TestTruncateEmoji(t *testing.T) {
	r := truncate("🔑🔐🏛️🖥️🏭", 4)
	for _, c := range r {
		if c == 0xFFFD {
			t.Fatal("broken UTF-8 in emoji truncation")
		}
	}
}

func TestTruncateEmpty(t *testing.T) {
	if r := truncate("", 10); r != "" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateMaxLenZero(t *testing.T) {
	// maxLen < 3 returns original
	if r := truncate("hello", 0); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateMaxLenOne(t *testing.T) {
	if r := truncate("hello", 1); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

// Domain: str helper returns correct map values
func TestStrHelper(t *testing.T) {
	m := map[string]interface{}{
		"name":  "test",
		"count": 42,
	}
	if s := str(m, "name"); s != "test" {
		t.Fatalf("got %q", s)
	}
	if s := str(m, "missing"); s != "" {
		t.Fatalf("got %q", s)
	}
}

// Domain: switchPage resets error
func TestSwitchPageResetsModel(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.err = fmt.Errorf("old error")
	m, _ = m.switchPage(pageKeycenter)
	if m.err != nil {
		t.Fatal("switchPage must clear m.err")
	}
}

// BUG-1: updateCreate must forward regular key input to the focused textinput
func TestUpdateCreate_ForwardsRegularKeyInput(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcCreate
	m.creating = true
	m.focusIdx = 0
	m.nameInput.Focus()

	// Send a regular character key — should be forwarded to the focused textinput
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	m2, _ := m.updateCreate(keyMsg, nil)

	if m2.nameInput.Value() != "a" {
		t.Errorf("expected nameInput to contain 'a', got %q", m2.nameInput.Value())
	}

	// Now test with focus on valueInput
	m.focusIdx = 1
	m.nameInput.Blur()
	m.valueInput.Focus()

	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	m3, _ := m.updateCreate(keyMsg, nil)

	if m3.valueInput.Value() != "x" {
		t.Errorf("expected valueInput to contain 'x', got %q", m3.valueInput.Value())
	}
}

// BUG-3: plugins must set loaded=true on error so "Loading..." disappears
func TestPlugins_LoadedTrueOnError(t *testing.T) {
	m := newPluginsModel()
	if m.loaded {
		t.Fatal("expected loaded=false initially")
	}

	m2, _ := m.update(errMsg{fmt.Errorf("connection refused")}, nil)

	if !m2.loaded {
		t.Error("expected loaded=true after errMsg")
	}
	if m2.err == "" {
		t.Error("expected err to be set after errMsg")
	}
}

// BUG-2: vaults errMsg must reset all loading states
func TestVaults_ErrMsgResetsAllLoadingStates(t *testing.T) {
	m := newVaultsModel()
	m.secretsLoading = true
	m.metaLoading = true
	m.revealing = true
	m.loading = true

	m2, _ := m.update(errMsg{fmt.Errorf("timeout")}, nil)

	if m2.secretsLoading {
		t.Error("expected secretsLoading=false after errMsg")
	}
	if m2.metaLoading {
		t.Error("expected metaLoading=false after errMsg")
	}
	if m2.revealing {
		t.Error("expected revealing=false after errMsg")
	}
	if m2.loading {
		t.Error("expected loading=false after errMsg")
	}
	if !m2.offline {
		t.Error("expected offline=true after errMsg")
	}
}

// Domain: all pages have switchPage case
func TestSwitchPageAllPages(t *testing.T) {
	m := NewModel("http://localhost:1")
	pages := []page{pageKeycenter, pageVaults, pageSettings, pageAudit, pagePlugins, pageFunctions}
	for _, p := range pages {
		result, _ := m.switchPage(p)
		if result.activePage != p {
			t.Fatalf("switchPage(%d) did not set page", p)
		}
	}
}
