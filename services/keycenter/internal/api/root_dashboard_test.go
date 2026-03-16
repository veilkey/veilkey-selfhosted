package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"veilkey-keycenter/internal/db"
)

func TestRootServesDashboardWhenUnlocked(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "VeilKey KeyCenter") {
		t.Fatalf("expected dashboard HTML, got %q", body)
	}
	if !strings.Contains(body, "Operations Console (Variation 9)") || !strings.Contains(body, "<div id=\"app\"></div>") || !strings.Contains(body, "/assets/") {
		t.Fatalf("expected root dashboard to serve the built admin shell")
	}
	if !strings.Contains(body, "/favicon.svg") {
		t.Fatalf("expected root dashboard to include favicon link")
	}
	if strings.Contains(body, "Unlock first to enter the operator console.") {
		t.Fatalf("expected unlocked root to skip locked landing")
	}
}

func TestRootServesLockedLandingWhenLocked(t *testing.T) {
	_, handler := setupLockedServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Unlock first to enter the operator console.") {
		t.Fatalf("expected locked landing HTML, got %q", body)
	}
}

func TestRootServesInstallGateWhenInstallIncomplete(t *testing.T) {
	srv, handler := setupTestServer(t)
	if err := srv.db.SaveInstallSession(&db.InstallSession{
		SessionID:           "install-1",
		Flow:                "wizard",
		PlannedStagesJSON:   `["language","bootstrap","custody"]`,
		CompletedStagesJSON: `["language"]`,
		LastStage:           "language",
	}); err != nil {
		t.Fatalf("save install session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "이 리눅스 서버에 VeilKey 설치") {
		t.Fatalf("expected install gate HTML, got %q", body)
	}
	if !strings.Contains(body, "Linux Quick Install") {
		t.Fatalf("expected first-install wizard copy, got %q", body)
	}
	if !strings.Contains(body, "/api/install/runtime-config") {
		t.Fatalf("expected runtime config wiring in install gate")
	}
	if !strings.Contains(body, "빠른 설치 저장") {
		t.Fatalf("expected quick install actions in install gate")
	}
	if strings.Contains(body, "VeilKey KeyCenter") {
		t.Fatalf("expected install gate instead of dashboard")
	}
}

func TestDashboardRedirectsToRoot(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/" {
		t.Fatalf("expected redirect to /, got %q", got)
	}
}

func TestLegacyUIRedirectsToRoot(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/" {
		t.Fatalf("expected redirect to /, got %q", got)
	}
}

func TestAdminVuePreviewServesHTML(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/preview/admin-vue", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "VeilKey KeyCenter") {
		t.Fatalf("expected preview title, got %q", body)
	}
	if !strings.Contains(body, "Operations Console (Variation 9)") || !strings.Contains(body, "<div id=\"app\"></div>") || !strings.Contains(body, "/assets/") {
		t.Fatalf("expected built admin shell in preview HTML")
	}
}

func TestOperatorShellRoutesServeHTML(t *testing.T) {
	_, handler := setupTestServer(t)

	paths := []string{
		"/vaults/all",
		"/vaults/list",
		"/vaults/keys",
		"/vaults/host",
		"/vaults/local",
		"/vaults/local/68773ee9",
		"/vaults/local/68773ee9?tab=items",
		"/vaults/local/68773ee9?tab=bulk-apply&view=items",
		"/vaults/local/68773ee9?tab=bulk-apply&view=workflow",
		"/functions/list",
		"/functions/bindings",
		"/functions/impact",
		"/functions/run",
		"/audit",
		"/settings/ui",
		"/settings/admin",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "Operations Console (Variation 9)") || !strings.Contains(body, "<div id=\"app\"></div>") || !strings.Contains(body, "/assets/") {
			t.Fatalf("%s: expected admin shell HTML", path)
		}
		if !strings.Contains(body, "/favicon.svg") {
			t.Fatalf("%s: expected admin shell to include favicon link", path)
		}
	}
}

func TestLegacyVaultRouteRedirectsToLocalVaultShell(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/vaults/861f91ae?tab=items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/vaults/local/861f91ae?tab=items" {
		t.Fatalf("expected redirect to local vault shell, got %q", got)
	}
}

func TestAdminHTMLOneShotPreviewServesHTML(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/preview/admin-html-only", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "VeilKey KeyCenter") {
		t.Fatalf("expected preview title, got %q", body)
	}
	if !strings.Contains(body, "Operations Console (Variation 9)") || !strings.Contains(body, "<div id=\"app\"></div>") || !strings.Contains(body, "/assets/") {
		t.Fatalf("expected built admin shell in preview HTML")
	}
}

func TestAdminMockupPreviewRoutesServeHTML(t *testing.T) {
	_, handler := setupTestServer(t)

	tests := []struct {
		path string
	}{
		{path: "/preview/mockups/dark"},
		{path: "/preview/mockups/amber"},
		{path: "/preview/mockups/mono"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", tt.path, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "VeilKey KeyCenter") || !strings.Contains(body, "Operations Console (Variation 9)") || !strings.Contains(body, "<div id=\"app\"></div>") {
			t.Fatalf("%s: expected built admin shell", tt.path)
		}
	}
}

func TestFaviconServedFromAdminStaticAssets(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "image/svg+xml") {
		t.Fatalf("expected svg content type, got %q", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Fatalf("expected favicon svg body")
	}
}
