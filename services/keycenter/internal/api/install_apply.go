package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"veilkey-keycenter/internal/db"
)

type installApplyState struct {
	Status         string     `json:"status"`
	LastError      string     `json:"last_error,omitempty"`
	LastStartedAt  *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
	LastProfile    string     `json:"last_profile,omitempty"`
	LastRoot       string     `json:"last_root,omitempty"`
	LastScript     string     `json:"last_script,omitempty"`
	LastCommand    []string   `json:"last_command,omitempty"`
	LastOutput     string     `json:"last_output,omitempty"`
}

type installApplyPayload struct {
	InstallEnabled bool              `json:"install_enabled"`
	InstallRunning bool              `json:"install_running"`
	ScriptPath     string            `json:"script_path,omitempty"`
	Workdir        string            `json:"workdir,omitempty"`
	Profile        string            `json:"profile,omitempty"`
	InstallRoot    string            `json:"install_root,omitempty"`
	State          installApplyState `json:"state"`
}

func installScriptPath(cfg *db.UIConfig) string {
	if cfg != nil && strings.TrimSpace(cfg.InstallScript) != "" {
		return filepath.Clean(strings.TrimSpace(cfg.InstallScript))
	}
	if value := strings.TrimSpace(os.Getenv("VEILKEY_INSTALL_SCRIPT")); value != "" {
		return filepath.Clean(value)
	}
	return ""
}

func installWorkdir(cfg *db.UIConfig) string {
	if cfg != nil && strings.TrimSpace(cfg.InstallWorkdir) != "" {
		return filepath.Clean(strings.TrimSpace(cfg.InstallWorkdir))
	}
	return strings.TrimSpace(os.Getenv("VEILKEY_INSTALL_WORKDIR"))
}

func installTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("VEILKEY_INSTALL_TIMEOUT")); raw != "" {
		if dur, err := time.ParseDuration(raw); err == nil {
			return dur
		}
	}
	return 45 * time.Minute
}

func installScriptAllowlist() []string {
	seen := map[string]bool{}
	var allowed []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		path := filepath.Clean(raw)
		if !seen[path] {
			seen[path] = true
			allowed = append(allowed, path)
		}
	}

	for _, token := range strings.FieldsFunc(os.Getenv("VEILKEY_INSTALL_SCRIPT_ALLOWLIST"), func(r rune) bool {
		return r == ':' || r == ',' || r == '\n'
	}) {
		add(token)
	}
	add(os.Getenv("VEILKEY_INSTALL_SCRIPT"))
	return allowed
}

func isAllowlistedInstallScript(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	for _, allowed := range installScriptAllowlist() {
		if path == allowed {
			return true
		}
	}
	return false
}

func installCommand(script string, cfg *db.UIConfig) []string {
	profile := strings.TrimSpace(cfg.InstallProfile)
	root := strings.TrimSpace(cfg.InstallRoot)
	if root == "" {
		root = "/"
	}
	if filepath.Base(script) == "install.sh" {
		return []string{script, "install-profile", profile, root}
	}
	return []string{script}
}

func trimCommandOutput(output []byte) string {
	const maxBytes = 16 * 1024
	if len(output) <= maxBytes {
		return strings.TrimSpace(string(output))
	}
	return strings.TrimSpace(string(output[len(output)-maxBytes:]))
}

func installHTTPClient(caPath string) *http.Client {
	caPath = strings.TrimSpace(caPath)
	if caPath == "" {
		return &http.Client{Timeout: 15 * time.Second}
	}
	client, err := NewTLSHTTPClient(caPath, false)
	if err != nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	client.Timeout = 15 * time.Second
	return client
}

func checkHealthEndpoint(client *http.Client, rawURL string) error {
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if rawURL == "" {
		return nil
	}
	resp, err := client.Get(rawURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("health check returned non-200")
	}
	return nil
}

func (s *Server) snapshotInstallApply() installApplyState {
	s.installMu.RLock()
	defer s.installMu.RUnlock()
	return s.installState
}

func (s *Server) setInstallApplyState(state installApplyState) {
	s.installMu.Lock()
	s.installState = state
	s.installMu.Unlock()
}

func (s *Server) handleGetInstallApply(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	state := s.snapshotInstallApply()
	payload := installApplyPayload{
		InstallEnabled: installScriptPath(cfg) != "" && isAllowlistedInstallScript(installScriptPath(cfg)),
		InstallRunning: state.Status == "running",
		ScriptPath:     installScriptPath(cfg),
		Workdir:        installWorkdir(cfg),
		Profile:        strings.TrimSpace(cfg.InstallProfile),
		InstallRoot:    strings.TrimSpace(cfg.InstallRoot),
		State:          state,
	}
	s.respondJSON(w, http.StatusOK, payload)
}

func (s *Server) handleRunInstallApply(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}

	scriptPath := installScriptPath(cfg)
	if scriptPath == "" {
		s.respondError(w, http.StatusServiceUnavailable, "install script is not configured")
		return
	}
	if !isAllowlistedInstallScript(scriptPath) {
		s.respondError(w, http.StatusForbidden, "install script is not in the server allowlist")
		return
	}
	if _, err := os.Stat(scriptPath); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "install script is not available")
		return
	}
	if strings.TrimSpace(cfg.InstallProfile) == "" {
		s.respondError(w, http.StatusBadRequest, "install_profile is required before apply")
		return
	}

	s.installMu.Lock()
	if s.installState.Status == "running" {
		s.installMu.Unlock()
		s.respondError(w, http.StatusConflict, "install apply is already running")
		return
	}
	startedAt := time.Now().UTC()
	command := installCommand(scriptPath, cfg)
	s.installState = installApplyState{
		Status:        "running",
		LastStartedAt: &startedAt,
		LastProfile:   strings.TrimSpace(cfg.InstallProfile),
		LastRoot:      strings.TrimSpace(cfg.InstallRoot),
		LastScript:    scriptPath,
		LastCommand:   append([]string(nil), command...),
	}
	s.installMu.Unlock()

	go s.runInstallApply(scriptPath, cfg)

	s.respondJSON(w, http.StatusAccepted, map[string]any{
		"status":          "started",
		"script_path":     scriptPath,
		"install_root":    strings.TrimSpace(cfg.InstallRoot),
		"install_profile": strings.TrimSpace(cfg.InstallProfile),
		"started_at":      startedAt,
		"command":         command,
	})
}

func (s *Server) runInstallApply(scriptPath string, cfg *db.UIConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout())
	defer cancel()

	command := installCommand(scriptPath, cfg)
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	if workdir := installWorkdir(cfg); workdir != "" {
		cmd.Dir = workdir
	}

	root := strings.TrimSpace(cfg.InstallRoot)
	if root == "" {
		root = "/"
	}

	cmd.Env = append(os.Environ(),
		"VEILKEY_INSTALL_PROFILE="+strings.TrimSpace(cfg.InstallProfile),
		"VEILKEY_INSTALL_ROOT="+root,
		"VEILKEY_INSTALL_KEYCENTER_URL="+strings.TrimSpace(cfg.KeycenterURL),
		"VEILKEY_INSTALL_LOCALVAULT_URL="+strings.TrimSpace(cfg.LocalvaultURL),
		"VEILKEY_KEYCENTER_URL="+strings.TrimSpace(cfg.KeycenterURL),
		"VEILKEY_LOCALVAULT_URL="+strings.TrimSpace(cfg.LocalvaultURL),
		"VEILKEY_TLS_CERT="+strings.TrimSpace(cfg.TLSCertPath),
		"VEILKEY_TLS_KEY="+strings.TrimSpace(cfg.TLSKeyPath),
		"VEILKEY_TLS_CA="+strings.TrimSpace(cfg.TLSCAPath),
	)

	s.markInstallApplyStarted()
	output, err := cmd.CombinedOutput()
	client := installHTTPClient(cfg.TLSCAPath)

	next := installApplyState{
		Status:         "succeeded",
		LastStartedAt:  s.snapshotInstallApply().LastStartedAt,
		LastFinishedAt: timePtr(time.Now().UTC()),
		LastProfile:    strings.TrimSpace(cfg.InstallProfile),
		LastRoot:       root,
		LastScript:     scriptPath,
		LastCommand:    append([]string(nil), command...),
		LastOutput:     trimCommandOutput(output),
	}

	if err == nil {
		if healthErr := checkHealthEndpoint(client, cfg.KeycenterURL); healthErr != nil {
			err = healthErr
		}
	}
	if err == nil {
		if healthErr := checkHealthEndpoint(client, cfg.LocalvaultURL); healthErr != nil {
			err = healthErr
		}
	}

	if err != nil {
		next.Status = "failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			next.LastError = "install apply command timed out"
		} else {
			next.LastError = err.Error()
		}
		s.setInstallApplyState(next)
		return
	}

	_ = s.markInstallApplyCompleted()
	s.setInstallApplyState(next)
}

func (s *Server) markInstallApplyStarted() {
	session, err := s.db.GetLatestInstallSession()
	if err != nil || session == nil {
		return
	}
	if strings.TrimSpace(session.LastStage) == "" || strings.EqualFold(strings.TrimSpace(session.LastStage), "language") {
		session.LastStage = "apply_started"
		_ = s.db.SaveInstallSession(session)
	}
}

func (s *Server) markInstallApplyCompleted() error {
	session, err := s.db.GetLatestInstallSession()
	if err != nil || session == nil {
		return err
	}
	planned := decodeStringList(session.PlannedStagesJSON)
	completed := decodeStringList(session.CompletedStagesJSON)
	done := map[string]bool{}
	for _, stage := range completed {
		stage = strings.TrimSpace(stage)
		if stage != "" {
			done[stage] = true
		}
	}
	for _, stage := range planned {
		stage = strings.TrimSpace(stage)
		if stage == "" || done[stage] {
			continue
		}
		completed = append(completed, stage)
		done[stage] = true
	}
	if !done["final_smoke"] {
		completed = append(completed, "final_smoke")
	}
	session.CompletedStagesJSON = encodeStringList(completed)
	session.LastStage = "final_smoke"
	return s.db.SaveInstallSession(session)
}

func timePtr(value time.Time) *time.Time {
	return &value
}
