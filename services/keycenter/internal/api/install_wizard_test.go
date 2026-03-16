package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

// setupInstallIncompleteServer creates a server that is unlocked but has an
// incomplete install session, so GET / should render the install wizard.
func setupInstallIncompleteServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	kek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey (KEK): %v", err)
	}
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey (DEK): %v", err)
	}
	encDEK, nonce, err := crypto.EncryptDEK(kek, dek)
	if err != nil {
		t.Fatalf("EncryptDEK: %v", err)
	}
	if err := database.SaveNodeInfo(&db.NodeInfo{
		NodeID:   crypto.GenerateUUID(),
		DEK:      encDEK,
		DEKNonce: nonce,
		Version:  1,
	}); err != nil {
		t.Fatalf("SaveNodeInfo: %v", err)
	}

	srv := NewServer(database, kek, []string{})

	// Save an incomplete install session: planned stages not all done.
	if err := database.SaveInstallSession(&db.InstallSession{
		SessionID:           crypto.GenerateUUID(),
		Version:             1,
		Language:            "en",
		Flow:                "wizard",
		DeploymentMode:      "host-service",
		InstallScope:        "host-only",
		BootstrapMode:       "email",
		MailTransport:       "smtp",
		PlannedStagesJSON:   `["language","bootstrap","custody","final_smoke"]`,
		CompletedStagesJSON: `["language"]`,
		LastStage:           "language",
	}); err != nil {
		t.Fatalf("SaveInstallSession: %v", err)
	}

	handler := srv.SetupRoutes()
	return srv, handler
}

// ---------------------------------------------------------------------------
// TestRootRoute_Locked: locked server → GET / returns lockedLandingHTML.
// ---------------------------------------------------------------------------
func TestRootRoute_Locked(t *testing.T) {
	_, handler := setupLockedServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()

	// lockedLandingHTML contains this unique phrase.
	if !strings.Contains(body, "Unlock first to enter the operator console.") {
		t.Fatalf("expected locked landing HTML with unlock prompt, got %q", body)
	}
	// It also contains the locked badge.
	if !strings.Contains(body, `class="badge locked"`) {
		t.Fatalf("expected locked badge in locked landing HTML")
	}
	// Must NOT contain the install wizard or admin app mount points.
	if strings.Contains(body, `id="install-app"`) {
		t.Fatalf("locked state must not serve install wizard")
	}
	if strings.Contains(body, `id="app"`) {
		t.Fatalf("locked state must not serve admin dashboard")
	}
}

// ---------------------------------------------------------------------------
// TestRootRoute_InstallIncomplete: unlocked, install NOT done → install wizard.
// ---------------------------------------------------------------------------
func TestRootRoute_InstallIncomplete(t *testing.T) {
	_, handler := setupInstallIncompleteServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected Content-Type text/html, got %q", ct)
	}

	body := w.Body.String()

	// The install wizard HTML embeds <div id="install-app"></div> as the Vue mount point.
	if !strings.Contains(body, `id="install-app"`) {
		t.Fatalf("expected install wizard mount point id=\"install-app\", got %q", body)
	}
	if !strings.Contains(body, `/assets/install-`) {
		t.Fatalf("expected install wizard assets under /assets/, got %q", body)
	}

	// Must NOT contain the admin app standalone mount point.
	// The admin UI uses <div id="app"></div> — make sure it's absent.
	if strings.Contains(body, `<div id="app"></div>`) {
		t.Fatalf("install wizard must not contain admin app mount point <div id=\"app\"></div>")
	}

	// Must NOT contain locked landing content.
	if strings.Contains(body, "Unlock first to enter the operator console.") {
		t.Fatalf("install wizard must not show locked landing")
	}
}

// ---------------------------------------------------------------------------
// TestRootRoute_InstallComplete: unlocked, install IS done → admin dashboard.
// ---------------------------------------------------------------------------
func TestRootRoute_InstallComplete(t *testing.T) {
	_, handler := setupTestServer(t) // setupTestServer creates a fully complete install session

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()

	// The admin Vue app uses <div id="app"></div> as its mount point.
	if !strings.Contains(body, `<div id="app"></div>`) {
		t.Fatalf("expected admin dashboard with <div id=\"app\"></div>, got %q", body)
	}

	// Confirm it's the admin UI by checking for the title.
	if !strings.Contains(body, "Operations Console (Variation 9)") {
		t.Fatalf("expected admin UI title 'Operations Console (Variation 9)' in body")
	}

	// Must NOT contain install wizard mount point.
	if strings.Contains(body, `id="install-app"`) {
		t.Fatalf("complete install must not serve install wizard")
	}

	// Must NOT contain locked landing.
	if strings.Contains(body, "Unlock first to enter the operator console.") {
		t.Fatalf("complete install must not show locked landing")
	}
}

// ---------------------------------------------------------------------------
// TestRootRoute_NotFound: non-root path returns 404.
// ---------------------------------------------------------------------------
func TestRootRoute_NotFound(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /nonexistent, got %d", w.Code)
	}
}
