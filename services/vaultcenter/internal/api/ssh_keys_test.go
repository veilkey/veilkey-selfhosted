package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"veilkey-vaultcenter/internal/db"
)

func sshServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { database.Close() })
	if err := database.SetAdminPassword("test-pw!"); err != nil { t.Fatal(err) }
	kek := make([]byte, 32)
	for i := range kek { kek[i] = byte(i) }
	srv := NewServer(database, kek, []string{"127.0.0.1"})
	srv.SetSalt([]byte("test-salt-32-bytes-for-handler!!"))
	h, err := srv.SetupRoutes()
	if err != nil { t.Fatal(err) }
	return srv, h
}

func addSSH(t *testing.T, d *db.DB, id string, st db.RefStatus) {
	t.Helper()
	if err := d.SaveRef(db.RefParts{Family: "VK", Scope: db.RefScopeSSH, ID: id}, "enc-"+id, 1, st, ""); err != nil {
		t.Fatal(err)
	}
}
func addTemp(t *testing.T, d *db.DB, id string) {
	t.Helper()
	exp := time.Now().Add(time.Hour)
	if err := d.SaveRefWithExpiry(db.RefParts{Family: "VK", Scope: db.RefScopeTemp, ID: id}, "enc", 1, db.RefStatusTemp, exp, id); err != nil {
		t.Fatal(err)
	}
}
func addLocal(t *testing.T, d *db.DB, id string) {
	t.Helper()
	if err := d.SaveRef(db.RefParts{Family: "VK", Scope: db.RefScopeLocal, ID: id}, "enc", 1, db.RefStatusActive, ""); err != nil {
		t.Fatal(err)
	}
}

func sshGET(h http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/ssh/keys", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
func sshDEL(h http.Handler, ref string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("DELETE", "/api/ssh/keys/"+ref, nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
func sshParse(t *testing.T, rec *httptest.ResponseRecorder) ([]map[string]any, int) {
	t.Helper()
	var b map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	cnt := int(b["count"].(float64))
	raw, _ := json.Marshal(b["ssh_keys"])
	var keys []map[string]any
	_ = json.Unmarshal(raw, &keys)
	return keys, cnt
}

// ── GET ────────────────────────────────────────────────────────

func TestSSH_List_Empty(t *testing.T) {
	_, h := sshServer(t)
	r := sshGET(h)
	if r.Code != 200 { t.Fatalf("status=%d", r.Code) }
	_, c := sshParse(t, r)
	if c != 0 { t.Errorf("count=%d", c) }
}

func TestSSH_List_ReturnsInserted(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "k001", db.RefStatusActive)
	addSSH(t, srv.db, "k002", db.RefStatusActive)
	keys, c := sshParse(t, sshGET(h))
	if c != 2 { t.Errorf("count=%d", c) }
	refs := map[string]bool{}
	for _, k := range keys { refs[k["ref"].(string)] = true }
	if !refs["VK:SSH:k001"] || !refs["VK:SSH:k002"] { t.Error("missing refs") }
}

func TestSSH_List_ExcludesNonSSH(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "ssh1", db.RefStatusActive)
	addTemp(t, srv.db, "tmp1")
	addLocal(t, srv.db, "loc1")
	_, c := sshParse(t, sshGET(h))
	if c != 1 { t.Errorf("count=%d want 1", c) }
}

func TestSSH_List_ExcludesArchivedRevoked(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "act1", db.RefStatusActive)
	addSSH(t, srv.db, "arc1", db.RefStatusArchive)
	addSSH(t, srv.db, "rev1", db.RefStatusRevoke)
	keys, c := sshParse(t, sshGET(h))
	if c != 1 { t.Errorf("count=%d", c) }
	if keys[0]["ref"] != "VK:SSH:act1" { t.Errorf("ref=%v", keys[0]["ref"]) }
}

func TestSSH_List_NoCiphertext(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "nc01", db.RefStatusActive)
	body := sshGET(h).Body.String()
	if strings.Contains(body, "enc-nc01") { t.Error("ciphertext value leaked") }
	if strings.Contains(body, "ciphertext") { t.Error("ciphertext field leaked") }
}

func TestSSH_List_CountMatchesKeys(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "cm1", db.RefStatusActive)
	addSSH(t, srv.db, "cm2", db.RefStatusActive)
	addSSH(t, srv.db, "cm3", db.RefStatusActive)
	keys, c := sshParse(t, sshGET(h))
	if c != len(keys) { t.Errorf("count=%d keys=%d", c, len(keys)) }
}

func TestSSH_List_ContentType(t *testing.T) {
	_, h := sshServer(t)
	if ct := sshGET(h).Header().Get("Content-Type"); ct != "application/json" { t.Errorf("ct=%q", ct) }
}

func TestSSH_List_ResponseFields(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "rf01", db.RefStatusActive)
	keys, _ := sshParse(t, sshGET(h))
	k := keys[0]
	for _, f := range []string{"ref", "status", "created_at"} {
		if _, ok := k[f]; !ok { t.Errorf("missing %s", f) }
	}
}

// ── DELETE ──────────────────────────────────────────────────────

func TestSSH_Del_Success(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "d001", db.RefStatusActive)
	r := sshDEL(h, "VK:SSH:d001")
	if r.Code != 200 { t.Fatalf("status=%d body=%s", r.Code, r.Body.String()) }
	var b map[string]any
	_ = json.NewDecoder(r.Body).Decode(&b)
	if b["deleted"] != "VK:SSH:d001" { t.Errorf("deleted=%v", b["deleted"]) }
	_, c := sshParse(t, sshGET(h))
	if c != 0 { t.Error("must be gone") }
}

func TestSSH_Del_NotFound(t *testing.T) {
	_, h := sshServer(t)
	if sshDEL(h, "VK:SSH:nope").Code != 404 { t.Error("want 404") }
}

func TestSSH_Del_WrongScope(t *testing.T) {
	srv, h := sshServer(t)
	addLocal(t, srv.db, "loc99")
	if sshDEL(h, "VK:LOCAL:loc99").Code != 400 { t.Error("want 400") }
}

func TestSSH_Del_DoubleDel(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "dd01", db.RefStatusActive)
	sshDEL(h, "VK:SSH:dd01")
	if sshDEL(h, "VK:SSH:dd01").Code != 404 { t.Error("second del want 404") }
}

func TestSSH_Del_NoSideEffect(t *testing.T) {
	srv, h := sshServer(t)
	addSSH(t, srv.db, "keep", db.RefStatusActive)
	addSSH(t, srv.db, "gone", db.RefStatusActive)
	sshDEL(h, "VK:SSH:gone")
	keys, c := sshParse(t, sshGET(h))
	if c != 1 || keys[0]["ref"] != "VK:SSH:keep" { t.Error("wrong key survived") }
}

// ── Lifecycle ──────────────────────────────────────────────────

func TestSSH_Lifecycle(t *testing.T) {
	srv, h := sshServer(t)
	// empty
	_, c := sshParse(t, sshGET(h)); if c != 0 { t.Fatal("start empty") }
	// add
	addSSH(t, srv.db, "lc1", db.RefStatusActive)
	addSSH(t, srv.db, "lc2", db.RefStatusActive)
	_, c = sshParse(t, sshGET(h)); if c != 2 { t.Fatalf("after add: %d", c) }
	// delete one
	sshDEL(h, "VK:SSH:lc1")
	keys, c := sshParse(t, sshGET(h))
	if c != 1 || keys[0]["ref"] != "VK:SSH:lc2" { t.Error("after delete") }
	// delete last
	sshDEL(h, "VK:SSH:lc2")
	_, c = sshParse(t, sshGET(h)); if c != 0 { t.Error("must be empty") }
}
