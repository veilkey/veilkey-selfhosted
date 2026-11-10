package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"veilkey-vaultcenter/internal/db"
)

func operatorTestRoutes(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	s, mux := newTestServer(t)
	s.hkmHandler.Register(mux, s.requireTrustedIP, s.requireReadyForOps, s.requireAdminAuth)
	s.adminHandler.Register(mux, s.requireReadyForOps, s.requireTrustedIP)
	s.pluginHandler.Register(mux, s.requireTrustedIP)
	return s, mux
}

func operatorTestCookie(t *testing.T, mux *http.ServeMux) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, loginRequest(testAdminPassword))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == adminSessionCookie {
			return c
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func TestOperatorPlaintextRoutesRequireAdminSession(t *testing.T) {
	_, mux := operatorTestRoutes(t)
	paths := []string{
		"/api/resolve/VK:TEMP:missing", "/api/resolve-agent/12345678missing",
		"/api/vaults/missing/keys/KEY", "/api/vaults/missing/keys/KEY/fields/FIELD",
		"/api/mask-map", "/api/configs/search/KEY",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			for _, remote := range []string{"127.0.0.1:12000", "203.0.113.7:12000"} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.RemoteAddr = remote
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				want := http.StatusUnauthorized
				if strings.HasPrefix(remote, "203.") {
					want = http.StatusForbidden
				}
				if rec.Code != want {
					t.Errorf("remote %s status = %d, want %d", remote, rec.Code, want)
				}
			}
		})
	}
}

func TestOperatorInventoryRejectsUntrustedIP(t *testing.T) {
	_, mux := operatorTestRoutes(t)
	for _, path := range []string{"/api/agents", "/api/vaults", "/api/vaults/missing", "/api/vaults/missing/keys"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "203.0.113.7:12000"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s status = %d, want 403", path, rec.Code)
		}
	}
}

func TestOperatorPlaintextRoutesAcceptAdminSession(t *testing.T) {
	s, mux := operatorTestRoutes(t)
	s.SetMaskCacheData([]byte(`{"version":1,"changed":true,"entries":[]}`))
	cookie := operatorTestCookie(t, mux)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/mask-map", http.StatusOK},
		{"/api/vaults/missing/keys/KEY", http.StatusNotFound},
		{"/api/resolve-agent/12345678missing", http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:12000"
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("%s status = %d, want %d", tc.path, rec.Code, tc.status)
		}
	}
}

func TestRebindLegacyRouteRequiresSessionAndEvidence(t *testing.T) {
	s, mux := operatorTestRoutes(t)
	now := time.Now().UTC()
	agent := &db.Agent{NodeID: "rebind-node", AgentHash: "a1b2c3d4", Label: "rebind", VaultHash: "rebind-vault", VaultName: "rebind", KeyVersion: 1, RebindRequired: true, BlockedAt: &now}
	if err := s.db.UpsertAgent(agent.NodeID, agent.Label, agent.VaultHash, agent.VaultName, "", 0, 0, 0, 1, 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpdateAgentDEK(agent.NodeID, agent.AgentHash, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpdateAgentStatePartial(agent.NodeID, &db.AgentStatePartialUpdate{RebindRequired: &agent.RebindRequired, SetBlockedAt: true, BlockedAt: &now}); err != nil {
		t.Fatal(err)
	}
	path := "/api/agents/a1b2c3d4/approve-rebind"
	payload := `{"confirm":"APPROVE A1B2C3D4","reason":"key context reviewed"}`
	for _, loggedIn := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12000"
		want := http.StatusUnauthorized
		if loggedIn {
			req.AddCookie(operatorTestCookie(t, mux))
			want = http.StatusConflict
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("session=%v status=%d, want %d", loggedIn, rec.Code, want)
		}
		stored, err := s.db.GetAgentByNodeID(agent.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		if !stored.RebindRequired || stored.BlockedAt == nil {
			t.Fatal("rebind state changed without approval evidence")
		}
	}
	if err := s.db.SaveApprovalTokenChallenge(&db.ApprovalTokenChallenge{Token: "rebind-evidence", TargetName: agent.AgentHash, Kind: "secure_input", Status: "submitted"}); err != nil {
		t.Fatal(err)
	}
	cookie := operatorTestCookie(t, mux)
	for _, body := range []string{`{}`, `{"confirm":"APPROVE A1B2C3D4"}`, payload} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12000"
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		want := http.StatusBadRequest
		if body == payload {
			want = http.StatusOK
		}
		if rec.Code != want {
			t.Fatalf("approval status=%d, want %d", rec.Code, want)
		}
	}
	stored, err := s.db.GetAgentByNodeID(agent.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RebindRequired || stored.BlockedAt != nil {
		t.Fatal("approved rebind state was not cleared")
	}
}

func TestAdminLoginLimitUsesTransportPeer(t *testing.T) {
	_, mux := newTestServer(t)
	peer := "192.168.1.42"
	clearLoginAttempts(peer)
	t.Cleanup(func() { clearLoginAttempts(peer) })
	for i := 0; i <= loginMaxAttempts; i++ {
		req := loginRequest("invalid-test-password")
		req.RemoteAddr = peer + ":12000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		req.Header.Set("X-Real-IP", fmt.Sprintf("203.0.113.%d", i+1))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		want := http.StatusUnauthorized
		if i == loginMaxAttempts {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("attempt %d status=%d, want %d", i+1, rec.Code, want)
		}
	}
}

func TestPluginAPIGetRejectsUntrustedIP(t *testing.T) {
	_, mux := operatorTestRoutes(t)
	req := httptest.NewRequest(http.MethodGet, "/api/plugins/missing/api/status", nil)
	req.RemoteAddr = "203.0.113.7:12000"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", rec.Code)
	}
}
