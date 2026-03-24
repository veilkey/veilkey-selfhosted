package tui

import (
	"fmt"
	"testing"
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
