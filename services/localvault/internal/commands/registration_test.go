package commands

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"veilkey-localvault/internal/api"
	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/agentapi"
	"github.com/veilkey/veilkey-go-package/crypto"
)

func encodedTestRegistration(t *testing.T, tokenID, centerURL string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"t": tokenID, "u": centerURL})
	if err != nil {
		t.Fatal(err)
	}
	return "vk_reg_" + base64.RawURLEncoding.EncodeToString(payload)
}

// Run initialization in a child process because command errors and setup restart use os.Exit.
func TestRegistrationInitProcess(t *testing.T) {
	mode := os.Getenv("VEILKEY_TEST_INIT_MODE")
	if mode == "" {
		return
	}
	if mode == "web" {
		dbPath := os.Getenv("VEILKEY_DB_PATH")
		database, err := db.New(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(map[string]string{
			"vaultcenter_url":    os.Getenv("VEILKEY_TEST_CENTER"),
			"registration_token": os.Getenv("VEILKEY_TEST_TOKEN"),
		})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/install/init", bytes.NewReader(body))
		handleInstallInit(w, r, database, dbPath, filepath.Dir(dbPath))
		if w.Code != http.StatusOK {
			t.Fatalf("initialization returned %d", w.Code)
		}
	} else {
		os.Args = []string{"veilkey-localvault", "init", "--root", "--center", os.Getenv("VEILKEY_TEST_CENTER")}
		if mode == "force" {
			os.Args = append(os.Args, "--force")
		}
		if token := os.Getenv("VEILKEY_TEST_TOKEN"); token != "" {
			os.Args = append(os.Args, "--token", token)
		}
		RunInit()
	}
	os.Exit(0)
}

func initTestProcess(t *testing.T, mode, dbPath, centerURL, token string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestRegistrationInitProcess$")
	cmd.Env = append(os.Environ(), "VEILKEY_TEST_INIT_MODE="+mode,
		"VEILKEY_DB_PATH="+dbPath, "VEILKEY_TEST_CENTER="+centerURL, "VEILKEY_TEST_TOKEN="+token)
	return cmd
}

func TestInitRequiresRegistrationTokenBeforeChangingState(t *testing.T) {
	for _, mode := range []string{"cli", "force"} {
		t.Run(mode, func(t *testing.T) {
			dataDir := t.TempDir()
			dbPath := filepath.Join(dataDir, "localvault.db")
			original := []byte("existing-database")
			if err := os.WriteFile(dbPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := initTestProcess(t, mode, dbPath, "https://vaultcenter.example", "")
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "registration token is required") {
				t.Fatal("init must reject a missing token even with --center or --force")
			}
			data, err := os.ReadFile(dbPath)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("rejected init changed existing database")
			}
			if fileExists(filepath.Join(dataDir, "salt")) {
				t.Fatal("rejected init created salt")
			}
		})
	}
}

func TestInstallInitRequiresRegistrationTokenBeforeChangingState(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "localvault.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	body := strings.NewReader(`{"vaultcenter_url":"https://vaultcenter.example"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/install/init", body)
	handleInstallInit(w, r, database, dbPath, dataDir)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "registration token is required") {
		t.Fatal("web initialization must reject a missing registration token")
	}
	if err := database.Ping(); err != nil {
		t.Fatal("rejected initialization must preserve the setup database")
	}
	if fileExists(filepath.Join(dataDir, "salt")) || fileExists(filepath.Join(dataDir, "vault_key")) {
		t.Fatal("rejected initialization created state")
	}
}

func TestPrepareRegistrationValidatesTokenAndCenter(t *testing.T) {
	tokenID := crypto.GenerateUUID()
	var requests atomic.Int32
	var status atomic.Int32
	status.Store(http.StatusOK)
	center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != agentapi.PathRegistrationTokenValidate+tokenID+"/validate" {
			t.Error("unexpected registration validation request")
		}
		w.WriteHeader(int(status.Load()))
	}))
	defer center.Close()
	for _, embeddedURL := range []string{"", center.URL + "/"} {
		token := encodedTestRegistration(t, tokenID, embeddedURL)
		id, url, err := prepareRegistration(token, center.URL+"/")
		if err != nil || id != tokenID || url != center.URL {
			t.Fatal("valid registration should resolve and validate")
		}
	}
	if requests.Load() != 2 {
		t.Fatal("each initialization must validate its registration token remotely")
	}
	status.Store(http.StatusForbidden)
	if _, _, err := prepareRegistration(encodedTestRegistration(t, tokenID, center.URL), ""); err == nil {
		t.Fatal("rejected token must not initialize")
	}
	for _, token := range []string{"", " ", "invalid", encodedTestRegistration(t, "", center.URL)} {
		if _, _, err := prepareRegistration(token, center.URL); err == nil {
			t.Fatal("missing or malformed token must fail")
		}
	}
	if _, _, err := prepareRegistration(encodedTestRegistration(t, tokenID, ""), ""); err == nil {
		t.Fatal("registration requires a VaultCenter URL")
	}
}

func TestInitializationPersistsRegistrationTokenForHeartbeat(t *testing.T) {
	for _, mode := range []string{"cli", "web"} {
		t.Run(mode, func(t *testing.T) {
			tokenID := crypto.GenerateUUID()
			var validated atomic.Bool
			center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != agentapi.PathRegistrationTokenValidate+tokenID+"/validate" {
					t.Error("unexpected token validation path")
				}
				validated.Store(true)
				w.WriteHeader(http.StatusOK)
			}))
			defer center.Close()
			dataDir := t.TempDir()
			dbPath := filepath.Join(dataDir, "localvault.db")
			cmd := initTestProcess(t, mode, dbPath, "", encodedTestRegistration(t, tokenID, center.URL))
			if _, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("valid initialization failed: %v", err)
			}
			if !validated.Load() {
				t.Fatal("init did not validate registration token")
			}
			password, err := os.ReadFile(filepath.Join(dataDir, "vault_key"))
			if err != nil {
				t.Fatal(err)
			}
			salt, err := os.ReadFile(filepath.Join(dataDir, "salt"))
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("VEILKEY_DB_KEY", api.DeriveDBKeyFromKEK(crypto.DeriveKEK(string(password), salt)))
			database, err := db.New(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()
			registration, err := database.GetConfig("VEILKEY_REGISTRATION_TOKEN")
			if err != nil || registration.Value != tokenID {
				t.Fatal("initialization must persist the token used by the first heartbeat")
			}
			url, err := database.GetConfig(db.ConfigKeyVaultcenterURL)
			if err != nil || url.Value != center.URL {
				t.Fatal("initialization must persist the token's VaultCenter URL")
			}
		})
	}
}
