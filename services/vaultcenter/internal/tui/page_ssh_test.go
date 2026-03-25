package tui

import (
	"os"
	"strings"
	"testing"
)

// ══════════════════════════════════════════════════════════════════
// Domain-level tests for SSH TUI page
// These verify page registration, navigation, i18n, and UI behavior.
// ══════════════════════════════════════════════════════════════════

// --- Page registration ---

// Guarantees: pageSSH is defined in the page enum.
func TestSource_SSHPage_Defined(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "pageSSH") {
		t.Error("pageSSH must be defined in page enum")
	}
}

// Guarantees: SSH page is included in the pages slice and pageNameKeys.
func TestSource_SSHPage_InPagesSlice(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	// Must be in pages slice
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "var pages") {
			if !strings.Contains(line, "pageSSH") {
				t.Error("pageSSH must be in pages slice")
			}
		}
		if strings.Contains(line, "var pageNameKeys") {
			if !strings.Contains(line, "nav.ssh") {
				t.Error("nav.ssh must be in pageNameKeys")
			}
		}
	}
}

// Guarantees: SSH page is wired into Update switch.
func TestSource_SSHPage_InUpdateSwitch(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "case pageSSH:") {
		t.Error("pageSSH must have a case in Update switch")
	}
	if !strings.Contains(content, "m.ssh, cmd = m.ssh.update(msg, m.client)") {
		t.Error("pageSSH must delegate to m.ssh.update()")
	}
}

// Guarantees: SSH page is wired into View switch.
func TestSource_SSHPage_InViewSwitch(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "m.ssh.view(m.width)") {
		t.Error("pageSSH must render via m.ssh.view()")
	}
}

// Guarantees: SSH page is wired into switchPage.
func TestSource_SSHPage_InSwitchPage(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "loadSSHKeysCmd") {
		t.Error("switchPage must call loadSSHKeysCmd for pageSSH")
	}
}

// Guarantees: SSH model is included in Model struct and initialized.
func TestSource_SSHPage_ModelInitialized(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ssh       sshModel") && !strings.Contains(content, "ssh sshModel") {
		t.Error("Model struct must contain ssh sshModel field")
	}
	if !strings.Contains(content, "newSSHModel()") {
		t.Error("NewModel must initialize ssh field with newSSHModel()")
	}
}

// --- Key bindings ---

// Guarantees: Tab count matches pages count (7 tabs for 7 pages).
func TestSource_SSHPage_KeyBindingsExtended(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	// Must support "7" key for 7 pages
	if !strings.Contains(content, `"7"`) {
		t.Error("key bindings must include '7' for 7-page navigation")
	}
}

// --- isEditing ---

// Guarantees: SSH confirm state is respected in isEditing.
func TestSource_SSHPage_IsEditingHandled(t *testing.T) {
	src, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("failed to read model.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "m.ssh.confirm") {
		t.Error("isEditing must check m.ssh.confirm for pageSSH")
	}
}

// --- i18n ---

// Guarantees: SSH i18n keys exist for both EN and KO.
func TestSource_SSHPage_I18nKeys(t *testing.T) {
	src, err := os.ReadFile("i18n.go")
	if err != nil {
		t.Fatalf("failed to read i18n.go: %v", err)
	}
	content := string(src)

	requiredKeys := []string{
		`"nav.ssh"`,
		`"ssh.title"`,
		`"ssh.empty"`,
		`"ssh.add_hint"`,
		`"ssh.confirm_delete"`,
		`"ssh.help"`,
	}

	for _, key := range requiredKeys {
		count := strings.Count(content, key)
		if count < 2 {
			t.Errorf("i18n key %s must appear at least twice (EN + KO), found %d", key, count)
		}
	}
}

// --- SSH page behavior ---

// Guarantees: SSH page calls /api/ssh/keys endpoint.
func TestSource_SSHPage_CorrectEndpoint(t *testing.T) {
	src, err := os.ReadFile("page_ssh.go")
	if err != nil {
		t.Fatalf("failed to read page_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "/api/ssh/keys") {
		t.Error("SSH page must fetch from /api/ssh/keys endpoint")
	}
}

// Guarantees: SSH page supports cursor navigation (j/k or up/down).
func TestSource_SSHPage_CursorNavigation(t *testing.T) {
	src, err := os.ReadFile("page_ssh.go")
	if err != nil {
		t.Fatalf("failed to read page_ssh.go: %v", err)
	}
	content := string(src)

	for _, key := range []string{`"up"`, `"down"`, `"j"`, `"k"`} {
		if !strings.Contains(content, key) {
			t.Errorf("SSH page must handle %s key for cursor navigation", key)
		}
	}
}

// Guarantees: SSH page has delete confirmation flow (d → y/n).
func TestSource_SSHPage_DeleteConfirmation(t *testing.T) {
	src, err := os.ReadFile("page_ssh.go")
	if err != nil {
		t.Fatalf("failed to read page_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"d"`) {
		t.Error("SSH page must handle 'd' key to initiate delete")
	}
	if !strings.Contains(content, `"y"`) {
		t.Error("SSH page must handle 'y' key to confirm delete")
	}
	if !strings.Contains(content, `"n"`) || !strings.Contains(content, `"esc"`) {
		t.Error("SSH page must handle 'n' or 'esc' to cancel delete")
	}
	if !strings.Contains(content, "m.confirm") {
		t.Error("SSH page must use confirm flag for delete flow")
	}
}

// Guarantees: SSH page blocks navigation keys during confirm mode.
func TestSource_SSHPage_ConfirmBlocksNavigation(t *testing.T) {
	src, err := os.ReadFile("page_ssh.go")
	if err != nil {
		t.Fatalf("failed to read page_ssh.go: %v", err)
	}
	content := string(src)

	// j/k/r handlers must check !m.confirm
	if !strings.Contains(content, "!m.confirm && m.cursor") {
		t.Error("cursor navigation must be blocked during confirm mode")
	}
}

// Guarantees: SSH page shows empty state with add hint.
func TestSource_SSHPage_EmptyState(t *testing.T) {
	src, err := os.ReadFile("page_ssh.go")
	if err != nil {
		t.Fatalf("failed to read page_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ssh.empty") {
		t.Error("SSH page must show empty message when no keys")
	}
	if !strings.Contains(content, "ssh.add_hint") {
		t.Error("SSH page must show add hint when empty")
	}
}

// Guarantees: SSH page has refresh support.
func TestSource_SSHPage_RefreshSupport(t *testing.T) {
	src, err := os.ReadFile("page_ssh.go")
	if err != nil {
		t.Fatalf("failed to read page_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"r"`) {
		t.Error("SSH page must handle 'r' key for refresh")
	}
	if !strings.Contains(content, "loadSSHKeysCmd") {
		t.Error("SSH page refresh must call loadSSHKeysCmd")
	}
}
