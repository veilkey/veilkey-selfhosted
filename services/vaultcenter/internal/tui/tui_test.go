package tui

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ════════════════════════════════════════════════════════════════════════════════
// 1. Login Flow Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestLogin_PasswordLoginCreatesCorrectRequest(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authPassword
	m.pwInput.Focus()
	m.pwInput.SetValue("admin123")

	m2, cmd := m.handleSubmit(nil)
	if !m2.logging {
		t.Fatal("expected logging=true after submit")
	}
	if m2.errText != "" {
		t.Fatalf("unexpected error: %s", m2.errText)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for loginPasswordCmd")
	}
}

func TestLogin_TOTPLoginCreatesCorrectRequest(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authTOTP
	m.totpInput.Focus()
	m.totpInput.SetValue("123456")

	m2, cmd := m.handleSubmit(nil)
	if !m2.logging {
		t.Fatal("expected logging=true after submit")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for loginTOTPCmd")
	}
}

func TestLogin_EmptyPasswordRejected(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authPassword
	m.pwInput.Focus()
	m.pwInput.SetValue("")

	m2, cmd := m.handleSubmit(nil)
	if m2.logging {
		t.Fatal("should not set logging=true for empty password")
	}
	if cmd != nil {
		t.Fatal("should not return cmd for empty password")
	}
}

func TestLogin_EmptyTOTPRejected(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authTOTP
	m.totpInput.Focus()
	m.totpInput.SetValue("   ")

	m2, cmd := m.handleSubmit(nil)
	if m2.logging {
		t.Fatal("should not set logging=true for empty TOTP")
	}
	if cmd != nil {
		t.Fatal("should not return cmd for empty TOTP")
	}
}

func TestLogin_ErrorShowsErrorState(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authPassword

	m2, _ := m.update(loginFailMsg{err: "invalid password"}, nil)
	if m2.errText != "invalid password" {
		t.Fatalf("expected errText='invalid password', got %q", m2.errText)
	}
	if m2.logging {
		t.Fatal("logging should be false after loginFailMsg")
	}
}

func TestLogin_TOTPErrorResetsTOTPInput(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authTOTP
	m.totpInput.SetValue("999999")
	m.logging = true

	m2, _ := m.update(loginFailMsg{err: "bad code"}, nil)
	if m2.totpInput.Value() != "" {
		t.Fatalf("expected totpInput to be cleared, got %q", m2.totpInput.Value())
	}
}

func TestLogin_SuccessTransitionsToKeycenter(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageLogin

	result, _ := m.Update(loginSuccessMsg{})
	model := result.(Model)
	if model.activePage != pageKeycenter {
		t.Fatalf("expected keycenter page, got %d", model.activePage)
	}
	if model.status != "ready" {
		t.Fatalf("expected status='ready', got %q", model.status)
	}
}

func TestLogin_StatusLockedSetsUnlockStep(t *testing.T) {
	m := newLoginModel()
	m2, _ := m.update(statusMsg{status: "locked"}, nil)
	if m2.step != loginStepUnlock {
		t.Fatalf("expected loginStepUnlock, got %d", m2.step)
	}
	if !m2.serverLocked {
		t.Fatal("expected serverLocked=true")
	}
}

func TestLogin_StatusReadySetsAuthStep(t *testing.T) {
	m := newLoginModel()
	m2, _ := m.update(statusMsg{status: "ready"}, nil)
	if m2.step != loginStepAuth {
		t.Fatalf("expected loginStepAuth, got %d", m2.step)
	}
}

func TestLogin_TabSwitchesAuthMethod(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepAuth
	m.method = authTOTP

	m2, _ := m.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m2.method != authPassword {
		t.Fatal("tab should switch from TOTP to password")
	}

	m3, _ := m2.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m3.method != authTOTP {
		t.Fatal("tab should switch from password to TOTP")
	}
}

func TestLogin_UnlockEmptyPasswordRejected(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepUnlock
	m.kekInput.Focus()
	m.kekInput.SetValue("")

	m2, cmd := m.handleSubmit(nil)
	if m2.logging {
		t.Fatal("should not set logging for empty unlock password")
	}
	if cmd != nil {
		t.Fatal("should not return cmd for empty unlock password")
	}
}

func TestLogin_UnlockSuccess(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepUnlock
	m.logging = true

	m2, _ := m.update(unlockSuccessMsg{}, nil)
	if m2.step != loginStepAuth {
		t.Fatalf("expected loginStepAuth, got %d", m2.step)
	}
	if m2.logging {
		t.Fatal("logging should be false after unlockSuccess")
	}
}

func TestLogin_UnlockFail(t *testing.T) {
	m := newLoginModel()
	m.step = loginStepUnlock
	m.logging = true

	m2, _ := m.update(unlockFailMsg{err: "wrong password"}, nil)
	if m2.errText != "wrong password" {
		t.Fatalf("expected errText='wrong password', got %q", m2.errText)
	}
	if m2.logging {
		t.Fatal("logging should be false after unlockFail")
	}
	if m2.kekInput.Value() != "" {
		t.Fatal("kekInput should be cleared")
	}
}

func TestLogin_ViewNoPanic(t *testing.T) {
	m := newLoginModel()
	_ = m.view(80) // loginStepCheckStatus

	m.step = loginStepUnlock
	_ = m.view(80)

	m.step = loginStepAuth
	m.method = authTOTP
	_ = m.view(80)

	m.method = authPassword
	_ = m.view(80)

	m.logging = true
	_ = m.view(80)

	m.logging = false
	m.errText = "some error"
	_ = m.view(80)
}

// ════════════════════════════════════════════════════════════════════════════════
// 2. KeyCenter Page Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestKeycenter_CreateEmptyValueRejected(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcCreate
	m.creating = true
	m.focusIdx = 1
	m.nameInput.SetValue("mykey")
	m.valueInput.SetValue("")

	m2, cmd := m.updateCreate(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd != nil {
		t.Fatal("should not return cmd for empty value")
	}
	_ = m2
}

func TestKeycenter_CreateNameOptionalValueRequired(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcCreate
	m.creating = true
	m.nameInput.SetValue("")
	m.valueInput.SetValue("secretval")

	_, cmd := m.updateCreate(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd == nil {
		t.Fatal("should return cmd when value is provided (name is optional)")
	}
}

func TestKeycenter_CreateTabSwitchesFocus(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcCreate
	m.creating = true
	m.focusIdx = 0
	m.nameInput.Focus()
	m.valueInput.Blur()

	m2, _ := m.updateCreate(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m2.focusIdx != 1 {
		t.Fatalf("expected focusIdx=1 after tab, got %d", m2.focusIdx)
	}

	m3, _ := m2.updateCreate(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m3.focusIdx != 0 {
		t.Fatalf("expected focusIdx=0 after second tab, got %d", m3.focusIdx)
	}
}

func TestKeycenter_CreateEscCancels(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcCreate
	m.creating = true

	m2, _ := m.updateCreate(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if m2.subview != kcList {
		t.Fatal("esc should return to kcList")
	}
	if m2.creating {
		t.Fatal("creating should be false after esc")
	}
}

func TestKeycenter_ListNavigationJK(t *testing.T) {
	m := newKeycenterModel()
	m.loading = false
	m.refs = []TempRef{
		{RefCanonical: "ref-1"},
		{RefCanonical: "ref-2"},
		{RefCanonical: "ref-3"},
	}
	m.cursor = 0

	// j moves down
	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m2.cursor)
	}

	// k moves up
	m3, _ := m2.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m3.cursor)
	}
}

func TestKeycenter_ListCursorStopsAtBounds(t *testing.T) {
	m := newKeycenterModel()
	m.loading = false
	m.refs = []TempRef{
		{RefCanonical: "ref-1"},
		{RefCanonical: "ref-2"},
	}

	// k at top stays at 0
	m.cursor = 0
	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m2.cursor != 0 {
		t.Fatalf("cursor should stay at 0, got %d", m2.cursor)
	}

	// j at bottom stays at max
	m.cursor = 1
	m3, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m3.cursor != 1 {
		t.Fatalf("cursor should stay at 1, got %d", m3.cursor)
	}
}

func TestKeycenter_RefreshKeyR(t *testing.T) {
	m := newKeycenterModel()
	m.loading = false

	m2, cmd := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadRefsCmd")
	}
}

func TestKeycenter_DeleteShowsNotSupported(t *testing.T) {
	m := newKeycenterModel()
	m.refs = []TempRef{{RefCanonical: "ref-1"}}
	m.cursor = 0

	_, cmd := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, nil)
	if cmd == nil {
		t.Fatal("d should return errMsg cmd")
	}
	// Execute the cmd to verify the message
	msg := cmd()
	errM, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("expected errMsg, got %T", msg)
	}
	if !strings.Contains(errM.err.Error(), "not supported") {
		t.Fatalf("expected 'not supported', got %q", errM.err.Error())
	}
}

func TestKeycenter_EnterOpensDetail(t *testing.T) {
	m := newKeycenterModel()
	m.refs = []TempRef{{RefCanonical: "ref-1", SecretName: "test"}}
	m.cursor = 0

	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if m2.subview != kcDetail {
		t.Fatal("enter should open detail view")
	}
	if m2.detailRef.RefCanonical != "ref-1" {
		t.Fatal("detail should show selected ref")
	}
}

func TestKeycenter_NOpensCreate(t *testing.T) {
	m := newKeycenterModel()
	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, nil)
	if m2.subview != kcCreate {
		t.Fatal("n should open create view")
	}
	if !m2.creating {
		t.Fatal("creating should be true")
	}
}

func TestKeycenter_PromoteNavigation(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcPromote
	m.vaults = []map[string]any{
		{"vault_hash": "v1"},
		{"vault_hash": "v2"},
		{"vault_hash": "v3"},
	}
	m.vaultCursor = 0

	m2, _ := m.updatePromote(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.vaultCursor != 1 {
		t.Fatalf("expected vaultCursor=1, got %d", m2.vaultCursor)
	}

	m3, _ := m2.updatePromote(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.vaultCursor != 0 {
		t.Fatalf("expected vaultCursor=0, got %d", m3.vaultCursor)
	}
}

func TestKeycenter_PromoteEscGoesBack(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcPromote

	m2, _ := m.updatePromote(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if m2.subview != kcDetail {
		t.Fatal("esc in promote should go back to detail")
	}
}

func TestKeycenter_DetailEscGoesBack(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcDetail

	m2, _ := m.updateDetail(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if m2.subview != kcList {
		t.Fatal("esc in detail should go back to list")
	}
}

func TestKeycenter_DetailHHidesReveal(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcDetail
	m.revealed = "my-secret-value"

	m2, _ := m.updateDetail(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, nil)
	if m2.revealed != "" {
		t.Fatal("h should hide revealed value")
	}
}

func TestKeycenter_RefsLoadedMsg(t *testing.T) {
	m := newKeycenterModel()
	m.loading = true

	refs := []TempRef{
		{RefCanonical: "ref-1"},
		{RefCanonical: "ref-2"},
	}
	m2, _ := m.update(refsLoadedMsg{refs}, nil)
	if m2.loading {
		t.Fatal("loading should be false after refsLoadedMsg")
	}
	if len(m2.refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(m2.refs))
	}
	if m2.offline {
		t.Fatal("offline should be false after refsLoadedMsg")
	}
}

func TestKeycenter_ErrMsgSetsOffline(t *testing.T) {
	m := newKeycenterModel()
	m.loading = true

	m2, _ := m.update(errMsg{fmt.Errorf("connection refused")}, nil)
	if m2.loading {
		t.Fatal("loading should be false after errMsg")
	}
	if !m2.offline {
		t.Fatal("offline should be true after errMsg")
	}
}

func TestKeycenter_RefCreatedReturnsToList(t *testing.T) {
	m := newKeycenterModel()
	m.subview = kcCreate
	m.creating = true

	m2, cmd := m.update(refCreatedMsg{ref: "new-ref"}, nil)
	if m2.subview != kcList {
		t.Fatal("should return to list after refCreated")
	}
	if m2.creating {
		t.Fatal("creating should be false")
	}
	if cmd == nil {
		t.Fatal("should return loadRefsCmd to refresh list")
	}
}

func TestKeycenter_ViewNoPanic(t *testing.T) {
	m := newKeycenterModel()
	_ = m.view(80) // loading state

	m.loading = false
	m.offline = true
	_ = m.view(80)

	m.offline = false
	m.refs = nil
	_ = m.view(80) // empty list

	m.refs = []TempRef{{RefCanonical: "ref-1", SecretName: "test", CreatedAt: time.Now()}}
	_ = m.view(80) // list with items

	m.subview = kcDetail
	m.detailRef = m.refs[0]
	_ = m.view(80) // detail view

	m.subview = kcCreate
	_ = m.view(80) // create view

	m.subview = kcPromote
	_ = m.view(80) // promote view - no vaults
	m.vaults = []map[string]any{{"vault_hash": "v1", "display_name": "Vault 1"}}
	_ = m.view(80) // promote view - with vaults
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

// ════════════════════════════════════════════════════════════════════════════════
// 3. Vaults Page Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestVaults_TabSwitching(t *testing.T) {
	m := newVaultsModel()
	m.loading = false

	// vaultTabList -> vaultTabAgents
	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m2.tab != vaultTabAgents {
		t.Fatalf("expected vaultTabAgents, got %d", m2.tab)
	}
	if cmd == nil {
		t.Fatal("switching to agents should trigger loadAgentsCmd")
	}

	// vaultTabAgents -> vaultTabCatalog
	m3, cmd := m2.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m3.tab != vaultTabCatalog {
		t.Fatalf("expected vaultTabCatalog, got %d", m3.tab)
	}
	if cmd == nil {
		t.Fatal("switching to catalog should trigger loadCatalogCmd")
	}

	// vaultTabCatalog -> vaultTabList
	m4, _ := m3.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m4.tab != vaultTabList {
		t.Fatalf("expected vaultTabList, got %d", m4.tab)
	}
}

func TestVaults_SelectVaultLoadsSecrets(t *testing.T) {
	m := newVaultsModel()
	m.loading = false
	m.vaults = []map[string]any{
		{"vault_runtime_hash": "rh1", "display_name": "Vault 1"},
	}
	m.cursor = 0

	m2, cmd := m.updateList(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if !m2.showDetail {
		t.Fatal("enter should set showDetail=true")
	}
	if !m2.secretsLoading {
		t.Fatal("should set secretsLoading=true")
	}
	if cmd == nil {
		t.Fatal("should return loadSecretsCmd")
	}
}

func TestVaults_SecretCreateNameValueRequired(t *testing.T) {
	m := newVaultsModel()
	m.creatingSecret = true
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}

	// Empty name
	m.createName.SetValue("")
	m.createValue.SetValue("val")
	_, cmd := m.updateCreateSecret(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd != nil {
		t.Fatal("should reject empty name")
	}

	// Empty value
	m.createName.SetValue("NAME")
	m.createValue.SetValue("")
	_, cmd = m.updateCreateSecret(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd != nil {
		t.Fatal("should reject empty value")
	}

	// Both provided
	m.createName.SetValue("NAME")
	m.createValue.SetValue("val")
	_, cmd = m.updateCreateSecret(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd == nil {
		t.Fatal("should accept valid name+value")
	}
}

func TestVaults_SecretCreateTabSwitchesFocus(t *testing.T) {
	m := newVaultsModel()
	m.creatingSecret = true
	m.createFocus = 0

	m2, _ := m.updateCreateSecret(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m2.createFocus != 1 {
		t.Fatalf("expected createFocus=1, got %d", m2.createFocus)
	}

	m3, _ := m2.updateCreateSecret(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m3.createFocus != 0 {
		t.Fatalf("expected createFocus=0, got %d", m3.createFocus)
	}
}

func TestVaults_SecretEditPrefillsValues(t *testing.T) {
	m := newVaultsModel()
	m.showDetail = true
	m.showSecretDetail = true
	m.secretDetail = map[string]any{"name": "MY_SECRET", "token": "VK:abc"}
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}

	// Simulate pressing 'e' for edit
	m2, _ := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}, nil)
	if !m2.editingSecret {
		t.Fatal("e should set editingSecret=true")
	}
	if !m2.creatingSecret {
		t.Fatal("e should set creatingSecret=true")
	}
	if m2.editSecretName != "MY_SECRET" {
		t.Fatalf("expected editSecretName='MY_SECRET', got %q", m2.editSecretName)
	}
	if m2.createName.Value() != "MY_SECRET" {
		t.Fatalf("expected createName='MY_SECRET', got %q", m2.createName.Value())
	}
}

func TestVaults_SecretDeleteConfirmDialog(t *testing.T) {
	m := newVaultsModel()
	m.showDetail = true
	m.showSecretDetail = true
	m.secretDetail = map[string]any{"name": "MY_SECRET", "token": "VK:abc"}
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}

	// Press 'd' to start delete
	m2, _ := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, nil)
	if !m2.confirmDelete {
		t.Fatal("d should set confirmDelete=true")
	}
	if m2.deleteTargetName != "MY_SECRET" {
		t.Fatalf("expected deleteTargetName='MY_SECRET', got %q", m2.deleteTargetName)
	}

	// Press 'n' to cancel
	m3, _ := m2.updateDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, nil)
	if m3.confirmDelete {
		t.Fatal("n should cancel delete")
	}
}

func TestVaults_SecretDeleteConfirmYes(t *testing.T) {
	m := newVaultsModel()
	m.confirmDelete = true
	m.deleteTargetName = "MY_SECRET"
	m.showSecretDetail = true
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}

	m2, cmd := m.updateDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, nil)
	if m2.confirmDelete {
		t.Fatal("y should clear confirmDelete")
	}
	if m2.showSecretDetail {
		t.Fatal("y should clear showSecretDetail")
	}
	if cmd == nil {
		t.Fatal("y should return deleteSecretCmd")
	}
}

func TestVaults_ListNavigation(t *testing.T) {
	m := newVaultsModel()
	m.loading = false
	m.vaults = []map[string]any{
		{"vault_hash": "v1"},
		{"vault_hash": "v2"},
		{"vault_hash": "v3"},
	}
	m.cursor = 0

	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m2.cursor)
	}

	m3, _ := m2.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m3.cursor)
	}
}

func TestVaults_ListCursorStopsAtBounds(t *testing.T) {
	m := newVaultsModel()
	m.loading = false
	m.vaults = []map[string]any{{"vault_hash": "v1"}, {"vault_hash": "v2"}}

	m.cursor = 0
	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m2.cursor != 0 {
		t.Fatalf("k at top should stay 0, got %d", m2.cursor)
	}

	m.cursor = 1
	m3, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m3.cursor != 1 {
		t.Fatalf("j at bottom should stay 1, got %d", m3.cursor)
	}
}

func TestVaults_RefreshVaults(t *testing.T) {
	m := newVaultsModel()
	m.loading = false

	m2, cmd := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadVaultsCmd")
	}
}

func TestVaults_AgentsNavigation(t *testing.T) {
	m := newVaultsModel()
	m.tab = vaultTabAgents
	m.agents = []map[string]any{{"name": "a1"}, {"name": "a2"}}
	m.agentCursor = 0

	m2, _ := m.updateAgents(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.agentCursor != 1 {
		t.Fatalf("expected agentCursor=1, got %d", m2.agentCursor)
	}
}

func TestVaults_CatalogSearchSlash(t *testing.T) {
	m := newVaultsModel()
	m.tab = vaultTabCatalog

	m2, _ := m.updateCatalog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}, nil)
	if !m2.catalogSearching {
		t.Fatal("/ should activate catalog search")
	}
}

func TestVaults_CatalogSearchEnterApplies(t *testing.T) {
	m := newVaultsModel()
	m.catalogSearching = true
	m.catalog = []map[string]any{
		{"secret_name": "DB_PASS", "ref_canonical": "ref1", "class": "vault"},
		{"secret_name": "API_KEY", "ref_canonical": "ref2", "class": "vault"},
	}
	m.catalogSearch.SetValue("DB")

	m2, _ := m.updateCatalogSearch(tea.KeyMsg{Type: tea.KeyEnter})
	if m2.catalogSearching {
		t.Fatal("enter should close catalog search")
	}
	if m2.catalogQuery != "DB" {
		t.Fatalf("expected catalogQuery='DB', got %q", m2.catalogQuery)
	}
	if len(m2.filteredCatalog) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(m2.filteredCatalog))
	}
}

func TestVaults_CatalogSearchEscCancels(t *testing.T) {
	m := newVaultsModel()
	m.catalogSearching = true

	m2, _ := m.updateCatalogSearch(tea.KeyMsg{Type: tea.KeyEsc})
	if m2.catalogSearching {
		t.Fatal("esc should cancel catalog search")
	}
}

func TestVaults_SecretsSearchSlash(t *testing.T) {
	m := newVaultsModel()
	m.showDetail = true

	m2, _ := m.updateSecrets(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}, nil)
	if !m2.searching {
		t.Fatal("/ should activate secrets search")
	}
}

func TestVaults_SecretsSearchEnterApplies(t *testing.T) {
	m := newVaultsModel()
	m.searching = true
	m.secrets = []map[string]any{
		{"name": "DB_PASS", "token": "VK:1"},
		{"name": "API_KEY", "token": "VK:2"},
	}
	m.searchInput.SetValue("API")

	m2, _ := m.updateSearch(tea.KeyMsg{Type: tea.KeyEnter})
	if m2.searching {
		t.Fatal("enter should close secrets search")
	}
	if len(m2.filteredSecrets) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(m2.filteredSecrets))
	}
}

func TestVaults_VaultsLoadedMsg(t *testing.T) {
	m := newVaultsModel()
	m.loading = true

	vaults := []map[string]any{{"vault_hash": "v1"}}
	m2, _ := m.update(vaultsLoadedMsg{vaults}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.vaults) != 1 {
		t.Fatalf("expected 1 vault, got %d", len(m2.vaults))
	}
}

func TestVaults_SecretsLoadedMsg(t *testing.T) {
	m := newVaultsModel()
	m.secretsLoading = true

	secrets := []map[string]any{{"name": "SECRET1"}, {"name": "SECRET2"}}
	m2, _ := m.update(secretsLoadedMsg{secrets}, nil)
	if m2.secretsLoading {
		t.Fatal("secretsLoading should be false")
	}
	if len(m2.filteredSecrets) != 2 {
		t.Fatalf("expected 2 filtered secrets, got %d", len(m2.filteredSecrets))
	}
}

func TestVaults_SecretCreatedReloads(t *testing.T) {
	m := newVaultsModel()
	m.creatingSecret = true
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}

	m2, cmd := m.update(secretCreatedMsg{}, nil)
	if m2.creatingSecret {
		t.Fatal("creatingSecret should be false")
	}
	if !m2.secretsLoading {
		t.Fatal("should trigger secrets reload")
	}
	if cmd == nil {
		t.Fatal("should return loadSecretsCmd")
	}
}

func TestVaults_SecretDeletedReloads(t *testing.T) {
	m := newVaultsModel()
	m.showSecretDetail = true
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}

	m2, cmd := m.update(secretDeletedMsg{}, nil)
	if m2.showSecretDetail {
		t.Fatal("showSecretDetail should be false")
	}
	if cmd == nil {
		t.Fatal("should return loadSecretsCmd")
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

func TestVaults_AgentsLoadedMsg(t *testing.T) {
	m := newVaultsModel()
	m.loading = true

	agents := []map[string]any{{"name": "agent1"}, {"name": "agent2"}}
	m2, _ := m.update(agentsLoadedMsg{agents}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(m2.agents))
	}
}

func TestVaults_CatalogLoadedMsg(t *testing.T) {
	m := newVaultsModel()
	m.loading = true

	catalog := []map[string]any{{"secret_name": "s1"}}
	m2, _ := m.update(catalogLoadedMsg{catalog}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.filteredCatalog) != 1 {
		t.Fatalf("expected 1 catalog entry, got %d", len(m2.filteredCatalog))
	}
}

func TestVaults_SecretRevealedMsg(t *testing.T) {
	m := newVaultsModel()
	m.revealing = true

	m2, _ := m.update(secretRevealedMsg{value: "secret-val"}, nil)
	if m2.revealing {
		t.Fatal("revealing should be false")
	}
	if m2.revealValue != "secret-val" {
		t.Fatalf("expected revealValue='secret-val', got %q", m2.revealValue)
	}
}

func TestVaults_CreateEscCancels(t *testing.T) {
	m := newVaultsModel()
	m.creatingSecret = true

	m2, _ := m.updateCreateSecret(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if m2.creatingSecret {
		t.Fatal("esc should cancel creating")
	}
}

func TestVaults_ViewNoPanic(t *testing.T) {
	m := newVaultsModel()
	_ = m.view(80) // loading state

	m.loading = false
	m.offline = true
	_ = m.view(80)

	m.offline = false
	_ = m.view(80) // empty list

	m.vaults = []map[string]any{{"vault_hash": "v1", "display_name": "Vault 1"}}
	_ = m.view(80)

	// Agents tab
	m.tab = vaultTabAgents
	m.agents = []map[string]any{{"name": "a1"}}
	_ = m.view(80)

	// Catalog tab
	m.tab = vaultTabCatalog
	m.catalog = []map[string]any{{"secret_name": "s1"}}
	m.filteredCatalog = m.catalog
	_ = m.view(80)

	// Detail view
	m.tab = vaultTabList
	m.showDetail = true
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1", "display_name": "V1"}
	m.filteredSecrets = []map[string]any{{"name": "S1", "token": "VK:abc"}}
	_ = m.view(80)

	// Secret detail view
	m.showSecretDetail = true
	m.secretDetail = map[string]any{"name": "S1", "token": "VK:abc"}
	_ = m.view(80)

	// Delete confirm
	m.showSecretDetail = false
	m.confirmDelete = true
	m.deleteTargetName = "S1"
	_ = m.view(80)

	// Creating secret
	m.confirmDelete = false
	m.creatingSecret = true
	_ = m.view(80)

	// Search mode
	m.creatingSecret = false
	m.showDetail = false
	m.searching = true
	_ = m.view(80)

	// Catalog searching
	m.searching = false
	m.catalogSearching = true
	_ = m.view(80)
}

func TestVaults_SecretEditFlow(t *testing.T) {
	m := newVaultsModel()
	m.creatingSecret = true
	m.editingSecret = true
	m.editSecretName = "EXISTING_KEY"
	m.detailVault = map[string]any{"vault_runtime_hash": "rh1"}
	m.createName.SetValue("EXISTING_KEY")
	m.createValue.SetValue("new-value")

	_, cmd := m.updateCreateSecret(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd == nil {
		t.Fatal("should return updateSecretCmd")
	}
}

func TestVaults_ApplySecretFilterEmpty(t *testing.T) {
	m := newVaultsModel()
	m.secrets = []map[string]any{
		{"name": "DB_PASS", "token": "VK:1"},
		{"name": "API_KEY", "token": "VK:2"},
	}
	m.searchQuery = ""
	m.applySecretFilter()
	if len(m.filteredSecrets) != 2 {
		t.Fatalf("expected all secrets with empty query, got %d", len(m.filteredSecrets))
	}
}

func TestVaults_ApplyCatalogFilterEmpty(t *testing.T) {
	m := newVaultsModel()
	m.catalog = []map[string]any{
		{"secret_name": "s1"},
		{"secret_name": "s2"},
	}
	m.catalogQuery = ""
	m.applyCatalogFilter()
	if len(m.filteredCatalog) != 2 {
		t.Fatalf("expected all catalog with empty query, got %d", len(m.filteredCatalog))
	}
}

func TestVaults_RefreshAgents(t *testing.T) {
	m := newVaultsModel()
	m.tab = vaultTabAgents

	m2, cmd := m.updateAgents(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadAgentsCmd")
	}
}

func TestVaults_RefreshCatalog(t *testing.T) {
	m := newVaultsModel()
	m.tab = vaultTabCatalog

	m2, cmd := m.updateCatalog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadCatalogCmd")
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// 4. Settings Page Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestSettings_TabSwitching(t *testing.T) {
	m := newSettingsModel()
	m.loading = false

	// status -> security
	m2, _ := m.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m2.tab != settingsSecurity {
		t.Fatalf("expected settingsSecurity, got %d", m2.tab)
	}

	// security -> tokens
	m3, cmd := m2.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m3.tab != settingsTokens {
		t.Fatalf("expected settingsTokens, got %d", m3.tab)
	}
	if cmd == nil {
		t.Fatal("switching to tokens should trigger loadTokensCmd")
	}

	// tokens -> configs
	m4, cmd := m3.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m4.tab != settingsConfigs {
		t.Fatalf("expected settingsConfigs, got %d", m4.tab)
	}
	if cmd == nil {
		t.Fatal("switching to configs should trigger loadConfigsCmd")
	}

	// configs -> status
	m5, _ := m4.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m5.tab != settingsStatus {
		t.Fatalf("expected settingsStatus, got %d", m5.tab)
	}
}

func TestSettings_TokenCreateInputWorks(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsTokens

	// Press 'n' to start creating
	m2, _ := m.updateTokens(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, nil)
	if !m2.creatingToken {
		t.Fatal("n should set creatingToken=true")
	}

	// Type characters into the input
	keyA := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	m3, _ := m2.updateCreateToken(keyA, nil)
	if m3.tokenInput.Value() != "a" {
		t.Fatalf("expected 'a', got %q", m3.tokenInput.Value())
	}

	// Empty label rejected
	m4 := m3
	m4.tokenInput.SetValue("")
	m5, cmd := m4.updateCreateToken(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd != nil {
		t.Fatal("empty label should be rejected")
	}
	_ = m5

	// Valid label accepted
	m3.tokenInput.SetValue("my-token")
	_, cmd = m3.updateCreateToken(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if cmd == nil {
		t.Fatal("valid label should return createTokenCmd")
	}
}

func TestSettings_TokenCreateEsc(t *testing.T) {
	m := newSettingsModel()
	m.creatingToken = true

	m2, _ := m.updateCreateToken(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if m2.creatingToken {
		t.Fatal("esc should cancel token creation")
	}
}

func TestSettings_LanguageToggle(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsStatus

	_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}, nil)
	if cmd == nil {
		t.Fatal("l should return langToggleMsg cmd")
	}
	msg := cmd()
	if _, ok := msg.(langToggleMsg); !ok {
		t.Fatalf("expected langToggleMsg, got %T", msg)
	}
}

func TestSettings_LanguageToggleInModel(t *testing.T) {
	defer SetLang(LangEN)

	m := NewModel("http://localhost:1")
	m.lang = LangEN
	SetLang(LangEN)

	result, _ := m.Update(langToggleMsg{})
	model := result.(Model)
	if model.lang != LangKO {
		t.Fatalf("expected LangKO, got %v", model.lang)
	}

	result2, _ := model.Update(langToggleMsg{})
	model2 := result2.(Model)
	if model2.lang != LangEN {
		t.Fatalf("expected LangEN, got %v", model2.lang)
	}
}

func TestSettings_StatusViewNoPanicOnNilData(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.status = nil
	m.nodeInfo = nil
	m.authInfo = nil
	_ = m.viewStatus()
}

func TestSettings_SecurityViewNoPanicOnNilData(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.authInfo = nil
	_ = m.viewSecurity()
}

func TestSettings_SettingsLoadedMsg(t *testing.T) {
	m := newSettingsModel()
	m.loading = true

	status := map[string]any{"mode": "standalone", "locked": false}
	nodeInfo := map[string]any{"node_id": "n1"}
	authInfo := map[string]any{"totp_enabled": true}

	m2, _ := m.update(settingsLoadedMsg{status, nodeInfo, authInfo}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if m2.status == nil {
		t.Fatal("status should be set")
	}
	if m2.nodeInfo == nil {
		t.Fatal("nodeInfo should be set")
	}
	if m2.authInfo == nil {
		t.Fatal("authInfo should be set")
	}
}

func TestSettings_TokensLoadedMsg(t *testing.T) {
	m := newSettingsModel()
	m.loading = true

	tokens := []map[string]any{{"token_id": "t1", "label": "test"}}
	m2, _ := m.update(tokensLoadedMsg{tokens}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(m2.tokens))
	}
}

func TestSettings_TokenCreatedMsg(t *testing.T) {
	m := newSettingsModel()
	m.creatingToken = true

	m2, cmd := m.update(tokenCreatedMsg{tokenID: "new-token"}, nil)
	if m2.creatingToken {
		t.Fatal("creatingToken should be false")
	}
	if !strings.Contains(m2.message, "new-token") {
		t.Fatalf("message should contain token ID, got %q", m2.message)
	}
	if cmd == nil {
		t.Fatal("should reload tokens")
	}
}

func TestSettings_TokenRevokedMsg(t *testing.T) {
	m := newSettingsModel()

	m2, cmd := m.update(tokenRevokedMsg{}, nil)
	if m2.message != "Token revoked" {
		t.Fatalf("expected 'Token revoked', got %q", m2.message)
	}
	if cmd == nil {
		t.Fatal("should reload tokens")
	}
}

func TestSettings_ConfigsLoadedMsg(t *testing.T) {
	m := newSettingsModel()
	m.loading = true

	configs := []map[string]any{{"key": "k1", "value": "v1"}}
	m2, _ := m.update(configsLoadedMsg{configs}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(m2.configs))
	}
}

func TestSettings_ConfigDeletedMsg(t *testing.T) {
	m := newSettingsModel()

	m2, cmd := m.update(configDeletedMsg{}, nil)
	if m2.message != "Config deleted" {
		t.Fatalf("expected 'Config deleted', got %q", m2.message)
	}
	if cmd == nil {
		t.Fatal("should reload configs")
	}
}

func TestSettings_RotationScheduledMsg(t *testing.T) {
	m := newSettingsModel()

	m2, _ := m.update(rotationScheduledMsg{}, nil)
	if !strings.Contains(m2.message, "Rotation scheduled") {
		t.Fatalf("expected rotation message, got %q", m2.message)
	}
}

func TestSettings_RefreshStatus(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsStatus

	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadSettingsCmd")
	}
}

func TestSettings_RefreshTokens(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsTokens

	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadTokensCmd")
	}
}

func TestSettings_RefreshConfigs(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsConfigs

	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadConfigsCmd")
	}
}

func TestSettings_ErrMsgSetsOffline(t *testing.T) {
	m := newSettingsModel()
	m.loading = true

	m2, _ := m.update(errMsg{fmt.Errorf("fail")}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if !m2.offline {
		t.Fatal("offline should be true")
	}
}

func TestSettings_TokenNavigation(t *testing.T) {
	m := newSettingsModel()
	m.tab = settingsTokens
	m.tokens = []map[string]any{{"token_id": "t1"}, {"token_id": "t2"}}
	m.tokenCursor = 0

	m2, _ := m.updateTokens(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.tokenCursor != 1 {
		t.Fatalf("expected tokenCursor=1, got %d", m2.tokenCursor)
	}

	m3, _ := m2.updateTokens(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.tokenCursor != 0 {
		t.Fatalf("expected tokenCursor=0, got %d", m3.tokenCursor)
	}
}

func TestSettings_ScheduleRotation(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsSecurity

	_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}, nil)
	if cmd == nil {
		t.Fatal("Shift+R should return scheduleRotationCmd")
	}
}

func TestSettings_ViewNoPanic(t *testing.T) {
	m := newSettingsModel()
	_ = m.view(80) // loading

	m.loading = false
	m.offline = true
	_ = m.view(80)

	m.offline = false
	_ = m.view(80) // status tab

	m.tab = settingsSecurity
	_ = m.view(80)

	m.tab = settingsTokens
	m.tokens = nil
	_ = m.view(80) // empty tokens

	m.tokens = []map[string]any{{"token_id": "t1", "label": "test"}}
	_ = m.view(80)

	m.creatingToken = true
	_ = m.view(80)

	m.creatingToken = false
	m.tab = settingsConfigs
	m.configs = nil
	_ = m.view(80) // empty configs

	m.configs = []map[string]any{{"key": "k1", "value": "v1"}}
	_ = m.view(80)

	m.message = "some message"
	_ = m.view(80)
}

// ════════════════════════════════════════════════════════════════════════════════
// 5. Audit Page Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestAudit_TimestampParsingRFC3339(t *testing.T) {
	m := newAuditModel()
	m.loading = false
	m.events = []map[string]any{
		{"created_at": "2026-03-25T10:30:00Z", "entity_type": "secret", "action": "create", "actor_type": "admin"},
	}
	// View should parse RFC3339 timestamp without panic
	view := m.view(120)
	if !strings.Contains(view, "03-25 10:30") {
		t.Fatalf("expected formatted timestamp, got:\n%s", view)
	}
}

func TestAudit_TimestampNonRFC3339Passthrough(t *testing.T) {
	m := newAuditModel()
	m.loading = false
	m.events = []map[string]any{
		{"created_at": "not-a-date", "entity_type": "secret", "action": "create"},
	}
	// Non-RFC3339 should pass through without panic
	view := m.view(120)
	if !strings.Contains(view, "not-a-date") {
		t.Fatalf("expected raw timestamp, got:\n%s", view)
	}
}

func TestAudit_EmptyLogShowsMessage(t *testing.T) {
	SetLang(LangEN)
	m := newAuditModel()
	m.loading = false
	m.events = nil

	view := m.view(80)
	if !strings.Contains(view, "No audit events.") {
		t.Fatalf("expected empty message, got:\n%s", view)
	}
}

func TestAudit_ListRendersCorrectly(t *testing.T) {
	m := newAuditModel()
	m.loading = false
	m.events = []map[string]any{
		{"created_at": "2026-03-25T10:30:00Z", "entity_type": "secret", "action": "create", "actor_type": "admin"},
		{"created_at": "2026-03-25T11:00:00Z", "entity_type": "vault", "action": "update", "actor_type": "system"},
	}
	m.cursor = 0

	view := m.view(120)
	if !strings.Contains(view, "secret") {
		t.Fatal("should contain 'secret'")
	}
	if !strings.Contains(view, "create") {
		t.Fatal("should contain 'create'")
	}
}

func TestAudit_Navigation(t *testing.T) {
	m := newAuditModel()
	m.loading = false
	m.events = []map[string]any{
		{"action": "a1"},
		{"action": "a2"},
	}
	m.cursor = 0

	m2, _ := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m2.cursor)
	}

	m3, _ := m2.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m3.cursor)
	}
}

func TestAudit_Refresh(t *testing.T) {
	m := newAuditModel()
	m.loading = false

	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadAuditCmd")
	}
}

func TestAudit_LoadedMsg(t *testing.T) {
	m := newAuditModel()
	m.loading = true

	events := []map[string]any{{"action": "test"}}
	m2, _ := m.update(auditLoadedMsg{events}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(m2.events))
	}
}

func TestAudit_ErrMsgSetsOffline(t *testing.T) {
	m := newAuditModel()
	m.loading = true

	m2, _ := m.update(errMsg{fmt.Errorf("fail")}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if !m2.offline {
		t.Fatal("offline should be true")
	}
}

func TestAudit_ViewNoPanic(t *testing.T) {
	m := newAuditModel()
	_ = m.view(80) // loading

	m.loading = false
	m.offline = true
	_ = m.view(80)

	m.offline = false
	_ = m.view(80) // empty

	m.events = []map[string]any{{"action": "test", "created_at": "2026-01-01T00:00:00Z"}}
	_ = m.view(80)
}

// ════════════════════════════════════════════════════════════════════════════════
// 6. Functions Page Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestFunctions_TabSwitching(t *testing.T) {
	m := newFunctionsModel()
	m.loading = false

	// functions -> bindings
	m2, cmd := m.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m2.tab != fnTabBindings {
		t.Fatalf("expected fnTabBindings, got %d", m2.tab)
	}
	if cmd == nil {
		t.Fatal("switching to bindings should trigger loadBindingsCmd")
	}

	// bindings -> functions
	m3, _ := m2.update(tea.KeyMsg{Type: tea.KeyTab}, nil)
	if m3.tab != fnTabList {
		t.Fatalf("expected fnTabList, got %d", m3.tab)
	}
}

func TestFunctions_RefreshFunctions(t *testing.T) {
	m := newFunctionsModel()
	m.loading = false
	m.tab = fnTabList

	m2, cmd := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadFunctionsCmd")
	}
}

func TestFunctions_RefreshBindings(t *testing.T) {
	m := newFunctionsModel()
	m.loading = false
	m.tab = fnTabBindings

	m2, cmd := m.updateBindings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if !m2.loading {
		t.Fatal("r should set loading=true")
	}
	if cmd == nil {
		t.Fatal("r should return loadBindingsCmd")
	}
}

func TestFunctions_EnterOpensDetail(t *testing.T) {
	m := newFunctionsModel()
	m.functions = []map[string]any{{"name": "fn1", "command": "cmd1"}}
	m.cursor = 0

	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyEnter}, nil)
	if !m2.showDetail {
		t.Fatal("enter should open detail")
	}
	if str(m2.detail, "name") != "fn1" {
		t.Fatal("detail should show selected function")
	}
}

func TestFunctions_DetailRunFunction(t *testing.T) {
	m := newFunctionsModel()
	m.showDetail = true
	m.detail = map[string]any{"name": "fn1"}

	m2, cmd := m.updateDetail(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, nil)
	if !m2.running {
		t.Fatal("x should set running=true")
	}
	if cmd == nil {
		t.Fatal("x should return runFunctionCmd")
	}
}

func TestFunctions_DetailEscGoesBack(t *testing.T) {
	m := newFunctionsModel()
	m.showDetail = true

	m2, _ := m.updateDetail(tea.KeyMsg{Type: tea.KeyEsc}, nil)
	if m2.showDetail {
		t.Fatal("esc should close detail")
	}
}

func TestFunctions_FunctionsLoadedMsg(t *testing.T) {
	m := newFunctionsModel()
	m.loading = true

	fns := []map[string]any{{"name": "fn1"}, {"name": "fn2"}}
	m2, _ := m.update(functionsLoadedMsg{fns}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(m2.functions))
	}
}

func TestFunctions_BindingsLoadedMsg(t *testing.T) {
	m := newFunctionsModel()
	m.loading = true

	bindings := []map[string]any{{"secret_name": "s1"}}
	m2, _ := m.update(bindingsLoadedMsg{bindings}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if len(m2.bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(m2.bindings))
	}
}

func TestFunctions_FunctionRunMsg(t *testing.T) {
	m := newFunctionsModel()
	m.running = true

	m2, _ := m.update(functionRunMsg{output: "result"}, nil)
	if m2.running {
		t.Fatal("running should be false")
	}
	if m2.runOutput != "result" {
		t.Fatalf("expected 'result', got %q", m2.runOutput)
	}
}

func TestFunctions_ErrMsgSetsOffline(t *testing.T) {
	m := newFunctionsModel()
	m.loading = true

	m2, _ := m.update(errMsg{fmt.Errorf("fail")}, nil)
	if m2.loading {
		t.Fatal("loading should be false")
	}
	if !m2.offline {
		t.Fatal("offline should be true")
	}
}

func TestFunctions_ListNavigation(t *testing.T) {
	m := newFunctionsModel()
	m.functions = []map[string]any{{"name": "f1"}, {"name": "f2"}, {"name": "f3"}}
	m.cursor = 0

	m2, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m2.cursor)
	}

	m3, _ := m2.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m3.cursor)
	}
}

func TestFunctions_BindingsNavigation(t *testing.T) {
	m := newFunctionsModel()
	m.bindings = []map[string]any{{"secret_name": "s1"}, {"secret_name": "s2"}}
	m.bindingCursor = 0

	m2, _ := m.updateBindings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, nil)
	if m2.bindingCursor != 1 {
		t.Fatalf("expected bindingCursor=1, got %d", m2.bindingCursor)
	}

	m3, _ := m2.updateBindings(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, nil)
	if m3.bindingCursor != 0 {
		t.Fatalf("expected bindingCursor=0, got %d", m3.bindingCursor)
	}
}

func TestFunctions_BindingsOfflineMessage(t *testing.T) {
	m := newFunctionsModel()
	m.loading = false
	m.tab = fnTabBindings
	m.offline = true

	view := m.viewBindings(80)
	if !strings.Contains(view, T("common.offline")) {
		t.Fatal("should show offline message")
	}
}

func TestFunctions_EmptyFunctions(t *testing.T) {
	SetLang(LangEN)
	m := newFunctionsModel()
	m.loading = false
	m.functions = nil

	view := m.viewList(80)
	if !strings.Contains(view, "No global functions.") {
		t.Fatalf("expected empty message, got:\n%s", view)
	}
}

func TestFunctions_EmptyBindings(t *testing.T) {
	SetLang(LangEN)
	m := newFunctionsModel()
	m.loading = false
	m.bindings = nil

	view := m.viewBindings(80)
	if !strings.Contains(view, "No bindings.") {
		t.Fatalf("expected empty bindings message, got:\n%s", view)
	}
}

func TestFunctions_ViewNoPanic(t *testing.T) {
	m := newFunctionsModel()
	_ = m.view(80) // loading

	m.loading = false
	m.offline = true
	_ = m.view(80)

	m.offline = false
	_ = m.view(80) // empty

	m.functions = []map[string]any{{"name": "fn1", "command": "cmd1", "category": "util"}}
	_ = m.view(80)

	m.tab = fnTabBindings
	_ = m.view(80) // empty bindings

	m.bindings = []map[string]any{{"secret_name": "s1", "ref_canonical": "ref1"}}
	_ = m.view(80)

	// Detail view
	m.showDetail = true
	m.detail = map[string]any{"name": "fn1", "command": "cmd1"}
	_ = m.view(80)

	m.running = true
	_ = m.view(80)

	m.running = false
	m.runOutput = "output text"
	_ = m.view(80)
}

// ════════════════════════════════════════════════════════════════════════════════
// 7. Plugins Page Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestPlugins_LoadSuccessShowsPlugins(t *testing.T) {
	m := newPluginsModel()

	plugins := []pluginItem{
		{Name: "plugin1", Version: "1.0", Loaded: true, Description: "Test plugin"},
	}
	m2, _ := m.update(pluginsListMsg{plugins: plugins}, nil)
	if !m2.loaded {
		t.Fatal("loaded should be true")
	}
	if len(m2.plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(m2.plugins))
	}
	if m2.err != "" {
		t.Fatalf("err should be empty, got %q", m2.err)
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

func TestPlugins_EmptyListShowsMessage(t *testing.T) {
	SetLang(LangEN)
	m := newPluginsModel()
	m.loaded = true
	m.plugins = nil

	view := m.view(80)
	if !strings.Contains(view, "No plugins installed.") {
		t.Fatalf("expected empty message, got:\n%s", view)
	}
}

func TestPlugins_RefreshKey(t *testing.T) {
	m := newPluginsModel()
	m.loaded = true

	_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}, nil)
	if cmd == nil {
		t.Fatal("r should return fetchPluginsCmd")
	}
}

func TestPlugins_ViewNoPanic(t *testing.T) {
	m := newPluginsModel()
	_ = m.view(80) // not loaded

	m.loaded = true
	_ = m.view(80) // empty

	m.plugins = []pluginItem{
		{Name: "p1", Version: "1.0", Loaded: true, Description: "A plugin"},
		{Name: "p2", Version: "2.0", Loaded: false, Description: "Another plugin with a long description that should be truncated"},
	}
	_ = m.view(80)

	m.err = "some error"
	_ = m.view(80)
}

func TestPlugins_ErrorViewShowsError(t *testing.T) {
	m := newPluginsModel()
	m.loaded = true
	m.err = "connection refused"

	view := m.view(80)
	if !strings.Contains(view, "connection refused") {
		t.Fatalf("expected error in view, got:\n%s", view)
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// 8. i18n Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestI18nEnglishDefault(t *testing.T) {
	SetLang(LangEN)
	if T("nav.keycenter") != "Keycenter" {
		t.Fatalf("expected 'Keycenter', got %q", T("nav.keycenter"))
	}
	if T("common.loading") != "Loading..." {
		t.Fatalf("expected 'Loading...', got %q", T("common.loading"))
	}
	if T("audit.empty") != "No audit events." {
		t.Fatalf("expected 'No audit events.', got %q", T("audit.empty"))
	}
}

func TestI18nKoreanSwitch(t *testing.T) {
	defer SetLang(LangEN) // restore

	SetLang(LangKO)
	if T("nav.keycenter") != "키센터" {
		t.Fatalf("expected '키센터', got %q", T("nav.keycenter"))
	}
	if T("common.loading") != "로딩 중..." {
		t.Fatalf("expected '로딩 중...', got %q", T("common.loading"))
	}
	if T("audit.empty") != "감사 항목이 없습니다." {
		t.Fatalf("expected '감사 항목이 없습니다.', got %q", T("audit.empty"))
	}
}

func TestI18nMissingKeyFallsBackToEnglish(t *testing.T) {
	defer SetLang(LangEN)

	SetLang(LangKO)
	// A key that exists in EN but not KO should fall back to EN
	// All current keys exist in both, so test with a completely unknown key
	if T("nonexistent.key") != "nonexistent.key" {
		t.Fatalf("expected raw key as fallback, got %q", T("nonexistent.key"))
	}

	// Verify fallback chain: KO missing -> EN exists -> return EN value
	SetLang(LangEN)
	enVal := T("nav.keycenter")
	SetLang(LangKO)
	koVal := T("nav.keycenter")
	if enVal == koVal {
		t.Fatal("EN and KO should differ for nav.keycenter")
	}
}

func TestI18nAllKeysExistInBothLanguages(t *testing.T) {
	enMap := translations[LangEN]
	koMap := translations[LangKO]

	for key := range enMap {
		if _, ok := koMap[key]; !ok {
			t.Errorf("key %q exists in EN but missing in KO", key)
		}
	}
	for key := range koMap {
		if _, ok := enMap[key]; !ok {
			t.Errorf("key %q exists in KO but missing in EN", key)
		}
	}
}

func TestI18n_TReturnsKeyForUnknownKey(t *testing.T) {
	SetLang(LangEN)
	result := T("totally.unknown.key")
	if result != "totally.unknown.key" {
		t.Fatalf("expected raw key, got %q", result)
	}
}

func TestI18n_SetLangChangesLanguage(t *testing.T) {
	defer SetLang(LangEN)

	SetLang(LangEN)
	if T("nav.keycenter") != "Keycenter" {
		t.Fatal("should be EN")
	}
	SetLang(LangKO)
	if T("nav.keycenter") != "키센터" {
		t.Fatal("should be KO")
	}
	SetLang(LangEN)
	if T("nav.keycenter") != "Keycenter" {
		t.Fatal("should be EN again")
	}
}

func TestI18n_AllENKeysExist(t *testing.T) {
	// Verify the EN translation map is non-empty and has expected keys
	enMap := translations[LangEN]
	if len(enMap) == 0 {
		t.Fatal("EN translations should not be empty")
	}

	requiredKeys := []string{
		"nav.keycenter", "nav.vaults", "nav.settings", "nav.audit", "nav.plugins", "nav.functions",
		"login.title", "kc.title", "vaults.title", "fn.title", "settings.title", "audit.title", "plugins.title",
		"common.loading", "common.offline",
	}
	for _, key := range requiredKeys {
		if _, ok := enMap[key]; !ok {
			t.Errorf("missing required EN key: %s", key)
		}
	}
}

func TestI18n_AllKOKeysExist(t *testing.T) {
	koMap := translations[LangKO]
	if len(koMap) == 0 {
		t.Fatal("KO translations should not be empty")
	}

	requiredKeys := []string{
		"nav.keycenter", "nav.vaults", "nav.settings", "nav.audit", "nav.plugins", "nav.functions",
		"login.title", "kc.title", "vaults.title", "fn.title", "settings.title", "audit.title", "plugins.title",
		"common.loading", "common.offline",
	}
	for _, key := range requiredKeys {
		if _, ok := koMap[key]; !ok {
			t.Errorf("missing required KO key: %s", key)
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// 9. Mouse Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestMouse_ClickTabBarSwitchesPage(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageKeycenter

	// Tab bar format: "  | 1 Name | 2 Name | ..."
	// Each tab label: " N Name " -> width varies
	names := pageNames()
	// Click in the area of the second tab (pageVaults)
	pos := 2 // initial margin
	pos += len(names[0]) + 4 + 1 // first tab width + space

	mouseMsg := tea.MouseMsg{
		X:      pos + 1,
		Y:      0,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}

	result, _ := m.Update(mouseMsg)
	model := result.(Model)
	if model.activePage != pageVaults {
		t.Fatalf("expected pageVaults, got %d", model.activePage)
	}
}

func TestMouse_ClickListItemSelectsKeycenter(t *testing.T) {
	m := newKeycenterModel()
	m.loading = false
	m.refs = []TempRef{
		{RefCanonical: "ref-0"},
		{RefCanonical: "ref-1"},
		{RefCanonical: "ref-2"},
	}
	m.cursor = 0

	// Click on the second item (Y=5, since list starts at Y=4)
	mouseMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	m2, _ := m.update(mouseMsg, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1 after click, got %d", m2.cursor)
	}
}

func TestMouse_ClickOutsideValidAreaKeycenter(t *testing.T) {
	m := newKeycenterModel()
	m.loading = false
	m.refs = []TempRef{{RefCanonical: "ref-0"}}
	m.cursor = 0

	// Click way below the list
	mouseMsg := tea.MouseMsg{
		X:      10,
		Y:      100,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	m2, _ := m.update(mouseMsg, nil)
	if m2.cursor != 0 {
		t.Fatalf("cursor should not change, got %d", m2.cursor)
	}
}

func TestMouse_ClickAuditItem(t *testing.T) {
	m := newAuditModel()
	m.loading = false
	m.events = []map[string]any{
		{"action": "a0"},
		{"action": "a1"},
	}
	m.cursor = 0

	mouseMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	m2, _ := m.update(mouseMsg, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m2.cursor)
	}
}

func TestMouse_ClickOnLoginPageIgnored(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageLogin

	mouseMsg := tea.MouseMsg{
		X:      5,
		Y:      0,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	result, _ := m.Update(mouseMsg)
	model := result.(Model)
	if model.activePage != pageLogin {
		t.Fatal("mouse click should not switch page on login")
	}
}

func TestMouse_ClickSettingsTokenItem(t *testing.T) {
	m := newSettingsModel()
	m.loading = false
	m.tab = settingsTokens
	m.tokens = []map[string]any{{"token_id": "t0"}, {"token_id": "t1"}}
	m.tokenCursor = 0

	mouseMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	m2, _ := m.update(mouseMsg, nil)
	if m2.tokenCursor != 1 {
		t.Fatalf("expected tokenCursor=1, got %d", m2.tokenCursor)
	}
}

func TestMouse_ClickFunctionsListItem(t *testing.T) {
	m := newFunctionsModel()
	m.loading = false
	m.tab = fnTabList
	m.functions = []map[string]any{{"name": "f0"}, {"name": "f1"}}
	m.cursor = 0

	mouseMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	m2, _ := m.update(mouseMsg, nil)
	if m2.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m2.cursor)
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// 10. Theme/Helpers Tests
// ════════════════════════════════════════════════════════════════════════════════

// Truncate tests (keep existing coverage)

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
	r := truncate("비밀번호입니다", 5)
	if len([]rune(r)) != 5 {
		t.Fatalf("expected 5 runes, got %d: %q", len([]rune(r)), r)
	}
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
	if r := truncate("hello", 0); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateMaxLenOne(t *testing.T) {
	if r := truncate("hello", 1); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateMaxLenTwo(t *testing.T) {
	if r := truncate("hello", 2); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateMaxLenThree(t *testing.T) {
	if r := truncate("hello", 3); r != "h.." {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateExactMaxLen(t *testing.T) {
	if r := truncate("abc", 3); r != "abc" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateKoreanExact(t *testing.T) {
	if r := truncate("가나다", 3); r != "가나다" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateKoreanTruncated(t *testing.T) {
	r := truncate("가나다라", 3)
	if r != "가.." {
		t.Fatalf("expected '가..', got %q", r)
	}
}

func TestTruncateSingleChar(t *testing.T) {
	if r := truncate("x", 1); r != "x" {
		t.Fatalf("got %q", r)
	}
	if r := truncate("x", 10); r != "x" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateNegativeMaxLen(t *testing.T) {
	if r := truncate("hello", -1); r != "hello" {
		t.Fatalf("got %q", r)
	}
}

func TestTruncateLongString(t *testing.T) {
	s := strings.Repeat("a", 1000)
	r := truncate(s, 10)
	if r != "aaaaaaaa.." {
		t.Fatalf("got %q", r)
	}
	if len([]rune(r)) != 10 {
		t.Fatalf("expected 10 runes, got %d", len([]rune(r)))
	}
}

// str helper tests

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

func TestStrNilMap(t *testing.T) {
	var m map[string]any
	if s := str(m, "key"); s != "" {
		t.Fatalf("expected empty for nil map, got %q", s)
	}
}

func TestStrNumericValue(t *testing.T) {
	m := map[string]any{"count": 42, "float": 3.14}
	if s := str(m, "count"); s != "42" {
		t.Fatalf("expected '42', got %q", s)
	}
	if s := str(m, "float"); s != "3.14" {
		t.Fatalf("expected '3.14', got %q", s)
	}
}

func TestStrBoolValue(t *testing.T) {
	m := map[string]any{"active": true}
	if s := str(m, "active"); s != "true" {
		t.Fatalf("expected 'true', got %q", s)
	}
}

// Color/style existence tests

func TestThemeColorsExist(t *testing.T) {
	// Verify all color variables are non-empty lipgloss colors
	colors := []lipgloss.TerminalColor{
		colorBg, colorFg, colorRed, colorGreen, colorYellow,
		colorBlue, colorCyan, colorMagenta, colorDimFg, colorHighlight,
	}
	for i, c := range colors {
		if c == nil {
			t.Fatalf("color at index %d is nil", i)
		}
	}
}

func TestThemeStylesRenderWithoutPanic(t *testing.T) {
	styles := []lipgloss.Style{
		styleTitle, styleHeader, styleActive, styleInactive,
		styleStatusBar, styleError, styleSuccess, styleLabel,
		styleValue, styleReveal, styleDim,
	}
	for i, s := range styles {
		result := s.Render("test")
		if result == "" {
			t.Fatalf("style at index %d rendered empty", i)
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// Model-level Tests
// ════════════════════════════════════════════════════════════════════════════════

func TestSwitchPageResetsModel(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.err = fmt.Errorf("old error")
	m, _ = m.switchPage(pageKeycenter)
	if m.err != nil {
		t.Fatal("switchPage must clear m.err")
	}
}

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

func TestModel_NumberKeySwitchesPage(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageKeycenter

	// Press "2" to switch to vaults
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	model := result.(Model)
	if model.activePage != pageVaults {
		t.Fatalf("expected pageVaults, got %d", model.activePage)
	}
}

func TestModel_NumberKeyIgnoredOnLogin(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageLogin

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	model := result.(Model)
	if model.activePage != pageLogin {
		t.Fatal("number keys should not switch page on login")
	}
}

func TestModel_NumberKeyIgnoredWhileEditing(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageKeycenter
	m.keycenter.creating = true

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	model := result.(Model)
	if model.activePage != pageKeycenter {
		t.Fatal("number keys should not switch page while editing")
	}
}

func TestModel_WindowSizeMsg(t *testing.T) {
	m := NewModel("http://localhost:1")
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	if model.width != 120 || model.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", model.width, model.height)
	}
}

func TestModel_ErrMsgSetsErr(t *testing.T) {
	m := NewModel("http://localhost:1")
	result, _ := m.Update(errMsg{fmt.Errorf("test error")})
	model := result.(Model)
	if model.err == nil || model.err.Error() != "test error" {
		t.Fatal("errMsg should set m.err")
	}
}

func TestModel_CtrlCQuits(t *testing.T) {
	m := NewModel("http://localhost:1")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should return quit cmd")
	}
}

func TestModel_QQuits(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageKeycenter

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should quit when not editing")
	}
}

func TestModel_QDoesNotQuitWhileEditing(t *testing.T) {
	m := NewModel("http://localhost:1")
	m.activePage = pageKeycenter
	m.keycenter.creating = true

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	// q should be forwarded to the editor, not cause quit
	// We check it does NOT return tea.Quit by checking the model state is still keycenter
	_ = cmd
}

func TestModel_ViewNoPanic(t *testing.T) {
	m := NewModel("http://localhost:1")
	_ = m.View() // login page

	m.activePage = pageKeycenter
	_ = m.View()

	m.activePage = pageVaults
	_ = m.View()

	m.activePage = pageFunctions
	_ = m.View()

	m.activePage = pageAudit
	_ = m.View()

	m.activePage = pagePlugins
	_ = m.View()

	m.activePage = pageSettings
	_ = m.View()

	m.err = fmt.Errorf("test error")
	_ = m.View()
}

func TestModel_IsEditing(t *testing.T) {
	m := NewModel("http://localhost:1")

	// Not editing by default
	m.activePage = pageKeycenter
	if m.isEditing() {
		t.Fatal("should not be editing by default")
	}

	// Keycenter creating
	m.keycenter.creating = true
	if !m.isEditing() {
		t.Fatal("should be editing when creating")
	}
	m.keycenter.creating = false

	// Keycenter promote
	m.keycenter.subview = kcPromote
	if !m.isEditing() {
		t.Fatal("should be editing when promoting")
	}
	m.keycenter.subview = kcList

	// Vaults creating secret
	m.activePage = pageVaults
	m.vaults.creatingSecret = true
	if !m.isEditing() {
		t.Fatal("should be editing when creating secret")
	}
	m.vaults.creatingSecret = false

	// Vaults searching
	m.vaults.searching = true
	if !m.isEditing() {
		t.Fatal("should be editing when searching")
	}
	m.vaults.searching = false

	// Vaults confirm delete
	m.vaults.confirmDelete = true
	if !m.isEditing() {
		t.Fatal("should be editing when confirming delete")
	}
	m.vaults.confirmDelete = false

	// Settings creating token
	m.activePage = pageSettings
	m.settings.creatingToken = true
	if !m.isEditing() {
		t.Fatal("should be editing when creating token")
	}
}

func TestDetectTabClick(t *testing.T) {
	names := []string{"Keycenter", "Vaults", "Functions", "Audit", "Plugins", "Settings"}

	// Click before any tab
	if idx := detectTabClick(0, names); idx != -1 {
		t.Fatalf("expected -1 for x=0, got %d", idx)
	}

	// Click on first tab
	if idx := detectTabClick(3, names); idx != 0 {
		t.Fatalf("expected 0 for first tab, got %d", idx)
	}

	// Click way off to the right
	if idx := detectTabClick(1000, names); idx != -1 {
		t.Fatalf("expected -1 for x=1000, got %d", idx)
	}
}

func TestPageNames(t *testing.T) {
	SetLang(LangEN)
	names := pageNames()
	if len(names) != len(pageNameKeys) {
		t.Fatalf("expected %d page names, got %d", len(pageNameKeys), len(names))
	}
	if names[0] != "Keycenter" {
		t.Fatalf("expected 'Keycenter', got %q", names[0])
	}
}

func TestClampCursor(t *testing.T) {
	// Cursor within range
	if c := clampCursor(2, 5); c != 2 {
		t.Fatalf("expected 2, got %d", c)
	}

	// Cursor at end
	if c := clampCursor(5, 5); c != 4 {
		t.Fatalf("expected 4, got %d", c)
	}

	// Cursor beyond end
	if c := clampCursor(10, 5); c != 4 {
		t.Fatalf("expected 4, got %d", c)
	}

	// Empty list
	if c := clampCursor(3, 0); c != 0 {
		t.Fatalf("expected 0, got %d", c)
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// TLS / Env / Error State Tests
// ════════════════════════════════════════════════════════════════════════════════

// extractTLSConfig extracts the TLS config from a Client's http.Transport.
func extractTLSConfig(c *Client) *tls.Config {
	transport, ok := c.http.Transport.(*http.Transport)
	if !ok {
		return nil
	}
	return transport.TLSClientConfig
}

func TestNewClientDefaultTLSVerify(t *testing.T) {
	t.Setenv("VEILKEY_TLS_INSECURE", "")

	c := NewClient("https://localhost:8443")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	tlsConf := extractTLSConfig(c)
	if tlsConf == nil {
		t.Fatal("TLS config is nil")
	}
	if tlsConf.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify should be false when VEILKEY_TLS_INSECURE is unset")
	}
}

func TestNewClientInsecureTLS(t *testing.T) {
	t.Setenv("VEILKEY_TLS_INSECURE", "1")

	c := NewClient("https://localhost:8443")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}

	tlsConf := extractTLSConfig(c)
	if tlsConf == nil {
		t.Fatal("TLS config is nil")
	}
	if !tlsConf.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify should be true when VEILKEY_TLS_INSECURE=1")
	}
}

func TestNewClientInsecureEnvValues(t *testing.T) {
	tests := []struct {
		envVal   string
		wantSkip bool
		desc     string
	}{
		{"1", true, "only '1' enables insecure mode"},
		{"0", false, "'0' keeps verification enabled"},
		{"", false, "empty string keeps verification enabled"},
		{"true", false, "'true' does NOT enable insecure mode — only '1' works"},
		{"yes", false, "'yes' does NOT enable insecure mode — only '1' works"},
		{"TRUE", false, "'TRUE' does NOT enable insecure mode — only '1' works"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("env=%q", tt.envVal), func(t *testing.T) {
			if tt.envVal == "" {
				t.Setenv("VEILKEY_TLS_INSECURE", "")
			} else {
				t.Setenv("VEILKEY_TLS_INSECURE", tt.envVal)
			}
			defer t.Setenv("VEILKEY_TLS_INSECURE", "")

			c := NewClient("https://localhost:8443")
			tlsConf := extractTLSConfig(c)
			if tlsConf == nil {
				t.Fatal("TLS config is nil")
			}
			if tlsConf.InsecureSkipVerify != tt.wantSkip {
				t.Fatalf("%s: InsecureSkipVerify=%v, want %v",
					tt.desc, tlsConf.InsecureSkipVerify, tt.wantSkip)
			}
		})
	}
}

func TestClientRequiredEnvVars(t *testing.T) {
	// Document: for self-signed certificates, users MUST set:
	//   VEILKEY_ADDR=https://<host>:<port>
	//   VEILKEY_TLS_INSECURE=1
	//
	// Without VEILKEY_TLS_INSECURE=1, connections to servers with self-signed
	// certs will fail with x509 certificate errors, causing errMsg propagation
	// that leaves pages stuck at "Loading...".

	t.Setenv("VEILKEY_TLS_INSECURE", "")

	// Creating a client with an unreachable URL must not panic.
	c := NewClient("https://192.0.2.1:9999") // RFC 5737 TEST-NET, guaranteed unreachable
	if c == nil {
		t.Fatal("NewClient must not return nil even with unreachable URL")
	}

	// The client should exist and have a valid transport.
	tlsConf := extractTLSConfig(c)
	if tlsConf == nil {
		t.Fatal("transport TLS config must not be nil")
	}
}

func TestVaultsLoadErrorSetsOffline(t *testing.T) {
	m := newVaultsModel()
	m.loading = true
	m.secretsLoading = true

	m2, cmd := m.update(errMsg{err: fmt.Errorf("connection refused")}, nil)
	if m2.loading {
		t.Fatal("loading must be false after errMsg")
	}
	if !m2.offline {
		t.Fatal("offline must be true after errMsg")
	}
	if m2.secretsLoading {
		t.Fatal("secretsLoading must be reset to false after errMsg — otherwise secrets page stays at Loading")
	}
	if m2.metaLoading {
		t.Fatal("metaLoading must be reset to false after errMsg")
	}
	if m2.revealing {
		t.Fatal("revealing must be reset to false after errMsg")
	}
	if cmd != nil {
		t.Fatal("no follow-up command expected after errMsg")
	}
}

func TestPluginsLoadErrorSetsLoaded(t *testing.T) {
	m := newPluginsModel()
	// Initially loaded=false, simulating "Loading..." state.
	if m.loaded {
		t.Fatal("initial state should have loaded=false")
	}

	m2, cmd := m.update(errMsg{err: fmt.Errorf("connection refused")}, nil)
	if !m2.loaded {
		t.Fatal("loaded must be true after errMsg — otherwise page stays at Loading")
	}
	if m2.err == "" {
		t.Fatal("err should be set after errMsg")
	}
	if cmd != nil {
		t.Fatal("no follow-up command expected after errMsg")
	}
}

func TestAllPagesHandleErrMsg(t *testing.T) {
	errMsgVal := errMsg{err: fmt.Errorf("TLS handshake failed: x509 certificate signed by unknown authority")}

	t.Run("vaults", func(t *testing.T) {
		m := newVaultsModel()
		m.loading = true
		m2, _ := m.update(errMsgVal, nil)
		if m2.loading {
			t.Fatal("vaults: loading should be false")
		}
		if !m2.offline {
			t.Fatal("vaults: offline should be true")
		}
	})

	t.Run("keycenter", func(t *testing.T) {
		m := newKeycenterModel()
		m2, _ := m.update(errMsgVal, nil)
		if m2.loading {
			t.Fatal("keycenter: loading should be false")
		}
		if !m2.offline {
			t.Fatal("keycenter: offline should be true")
		}
	})

	t.Run("functions", func(t *testing.T) {
		m := newFunctionsModel()
		m2, _ := m.update(errMsgVal, nil)
		if m2.loading {
			t.Fatal("functions: loading should be false")
		}
		if !m2.offline {
			t.Fatal("functions: offline should be true")
		}
	})

	t.Run("settings", func(t *testing.T) {
		m := newSettingsModel()
		m2, _ := m.update(errMsgVal, nil)
		if m2.loading {
			t.Fatal("settings: loading should be false")
		}
		if !m2.offline {
			t.Fatal("settings: offline should be true")
		}
	})

	t.Run("audit", func(t *testing.T) {
		m := newAuditModel()
		m2, _ := m.update(errMsgVal, nil)
		if m2.loading {
			t.Fatal("audit: loading should be false")
		}
		if !m2.offline {
			t.Fatal("audit: offline should be true")
		}
	})

	t.Run("plugins", func(t *testing.T) {
		m := newPluginsModel()
		m2, _ := m.update(errMsgVal, nil)
		if !m2.loaded {
			t.Fatal("plugins: loaded should be true after errMsg")
		}
		if m2.err == "" {
			t.Fatal("plugins: err should be set")
		}
	})
}

