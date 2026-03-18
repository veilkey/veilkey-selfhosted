package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
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
	LastRunID      string     `json:"last_run_id,omitempty"`
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

type installValidateRequest struct {
	ConfirmDangerousRoot bool `json:"confirm_dangerous_root"`
}

type installValidationResult struct {
	Valid             bool     `json:"valid"`
	ResolvedProfile   string   `json:"resolved_profile,omitempty"`
	ResolvedRoot      string   `json:"resolved_root,omitempty"`
	ResolvedScript    string   `json:"resolved_script,omitempty"`
	ResolvedWorkdir   string   `json:"resolved_workdir,omitempty"`
	DangerousRoot     bool     `json:"dangerous_root"`
	NeedsConfirmation bool     `json:"needs_confirmation"`
	Warnings          []string `json:"warnings,omitempty"`
	Errors            []string `json:"errors,omitempty"`
	CommandPreview    []string `json:"command_preview,omitempty"`
}

type installRunPayload struct {
	RunID          string                  `json:"run_id"`
	RunKind        string                  `json:"run_kind"`
	Status         string                  `json:"status"`
	InstallProfile string                  `json:"install_profile"`
	InstallRoot    string                  `json:"install_root"`
	ScriptPath     string                  `json:"script_path"`
	Workdir        string                  `json:"workdir"`
	Command        []string                `json:"command"`
	Validation     installValidationResult `json:"validation"`
	OutputTail     string                  `json:"output_tail,omitempty"`
	LastError      string                  `json:"last_error,omitempty"`
	StartedAt      time.Time               `json:"started_at"`
	FinishedAt     *time.Time              `json:"finished_at,omitempty"`
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

func resolveInstallProfile(cfg *db.UIConfig) string {
	if profile := strings.TrimSpace(cfg.InstallProfile); profile != "" {
		return profile
	}
	return "linux-host"
}

func validateInstallConfig(cfg *db.UIConfig, req installValidateRequest) installValidationResult {
	result := installValidationResult{
		Valid:           true,
		ResolvedProfile: resolveInstallProfile(cfg),
		ResolvedRoot:    strings.TrimSpace(cfg.InstallRoot),
		ResolvedScript:  installScriptPath(cfg),
		ResolvedWorkdir: installWorkdir(cfg),
	}
	if result.ResolvedRoot == "" {
		result.ResolvedRoot = "/"
	}
	if result.ResolvedWorkdir == "" && result.ResolvedScript != "" {
		result.ResolvedWorkdir = filepath.Dir(result.ResolvedScript)
	}
	if result.ResolvedScript == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "install script is not configured")
	}
	if result.ResolvedScript != "" && !isAllowlistedInstallScript(result.ResolvedScript) {
		result.Valid = false
		result.Errors = append(result.Errors, "install script is not in the server allowlist")
	}
	if result.ResolvedScript != "" {
		if _, err := os.Stat(result.ResolvedScript); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, "install script is not available")
		}
	}
	if result.ResolvedProfile == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "install profile could not be resolved")
	}
	if result.ResolvedWorkdir == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "install workdir is not configured")
	}
	if result.ResolvedWorkdir != "" {
		if info, err := os.Stat(result.ResolvedWorkdir); err != nil || !info.IsDir() {
			result.Valid = false
			result.Errors = append(result.Errors, "install workdir is not available")
		}
	}
	result.DangerousRoot = isDangerousInstallRoot(result.ResolvedRoot)
	if result.DangerousRoot {
		result.Warnings = append(result.Warnings, "install_root targets the live filesystem root")
		if !req.ConfirmDangerousRoot {
			result.Valid = false
			result.NeedsConfirmation = true
			result.Errors = append(result.Errors, "dangerous install_root requires explicit confirmation")
		}
	}
	if strings.TrimSpace(cfg.VaultcenterURL) == "" {
		result.Warnings = append(result.Warnings, "vaultcenter_url is empty; post-install verification may be limited")
	}
	if strings.TrimSpace(cfg.LocalvaultURL) == "" {
		result.Warnings = append(result.Warnings, "localvault_url is empty; localvault health verification will be skipped")
	}
	if strings.TrimSpace(cfg.TLSCertPath) == "" || strings.TrimSpace(cfg.TLSKeyPath) == "" {
		result.Warnings = append(result.Warnings, "TLS cert/key not configured; services will start without HTTPS. Set VEILKEY_TLS_CERT and VEILKEY_TLS_KEY env vars to enable TLS.")
	}
	result.CommandPreview = installCommand(result.ResolvedScript, cfg)
	return result
}

func isDangerousInstallRoot(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	return root == "/" || root == ""
}

func installCommand(script string, cfg *db.UIConfig) []string {
	profile := resolveInstallProfile(cfg)
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
	client, err := newTLSHTTPClient(caPath)
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
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func encodeJSON(v any, fallback string) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(data)
}

func installRunToPayload(run *db.InstallRun) installRunPayload {
	command := []string{}
	_ = json.Unmarshal([]byte(run.CommandJSON), &command)
	validation := installValidationResult{}
	_ = json.Unmarshal([]byte(run.ValidationJSON), &validation)
	return installRunPayload{
		RunID:          run.RunID,
		RunKind:        run.RunKind,
		Status:         run.Status,
		InstallProfile: run.InstallProfile,
		InstallRoot:    run.InstallRoot,
		ScriptPath:     run.ScriptPath,
		Workdir:        run.Workdir,
		Command:        command,
		Validation:     validation,
		OutputTail:     run.OutputTail,
		LastError:      run.LastError,
		StartedAt:      run.StartedAt,
		FinishedAt:     run.FinishedAt,
	}
}

func (h *Handler) handleGetInstallApply(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.db.GetOrCreateUIConfig()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	state := h.snapshotState()
	payload := installApplyPayload{
		InstallEnabled: installScriptPath(cfg) != "" && isAllowlistedInstallScript(installScriptPath(cfg)),
		InstallRunning: state.Status == "running",
		ScriptPath:     installScriptPath(cfg),
		Workdir:        installWorkdir(cfg),
		Profile:        resolveInstallProfile(cfg),
		InstallRoot:    strings.TrimSpace(cfg.InstallRoot),
		State:          state,
	}
	respond(w, http.StatusOK, payload)
}

func (h *Handler) handleGetInstallRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.db.ListInstallRuns(20)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to list install runs")
		return
	}
	payload := make([]installRunPayload, 0, len(runs))
	for i := range runs {
		payload = append(payload, installRunToPayload(&runs[i]))
	}
	respond(w, http.StatusOK, map[string]any{"runs": payload})
}

func (h *Handler) handleValidateInstallApply(w http.ResponseWriter, r *http.Request) {
	var req installValidateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, err := h.db.GetOrCreateUIConfig()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	result := validateInstallConfig(cfg, req)
	now := time.Now().UTC()
	run := &db.InstallRun{
		RunID:          crypto.GenerateUUID(),
		RunKind:        "validate",
		Status:         "validated",
		InstallProfile: result.ResolvedProfile,
		InstallRoot:    result.ResolvedRoot,
		ScriptPath:     result.ResolvedScript,
		Workdir:        result.ResolvedWorkdir,
		CommandJSON:    encodeJSON(result.CommandPreview, "[]"),
		ValidationJSON: encodeJSON(result, "{}"),
		StartedAt:      now,
		CreatedAt:      now,
	}
	finishedAt := time.Now().UTC()
	run.FinishedAt = &finishedAt
	if !result.Valid {
		run.Status = "rejected"
		run.LastError = strings.Join(result.Errors, "; ")
	}
	_ = h.db.SaveInstallRun(run)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	respond(w, status, map[string]any{
		"validation": result,
		"run":        installRunToPayload(run),
	})
}

func (h *Handler) handleRunInstallApply(w http.ResponseWriter, r *http.Request) {
	var req installValidateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, err := h.db.GetOrCreateUIConfig()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	validation := validateInstallConfig(cfg, req)
	if !validation.Valid {
		respond(w, http.StatusBadRequest, map[string]any{"validation": validation})
		return
	}

	h.mu.Lock()
	if h.state.Status == "running" {
		h.mu.Unlock()
		respondErr(w, http.StatusConflict, "install apply is already running")
		return
	}
	startedAt := time.Now().UTC()
	runID := crypto.GenerateUUID()
	h.state = installApplyState{
		Status:        "running",
		LastStartedAt: &startedAt,
		LastProfile:   validation.ResolvedProfile,
		LastRoot:      validation.ResolvedRoot,
		LastScript:    validation.ResolvedScript,
		LastCommand:   append([]string(nil), validation.CommandPreview...),
		LastRunID:     runID,
	}
	h.mu.Unlock()

	run := &db.InstallRun{
		RunID:          runID,
		RunKind:        "apply",
		Status:         "running",
		InstallProfile: validation.ResolvedProfile,
		InstallRoot:    validation.ResolvedRoot,
		ScriptPath:     validation.ResolvedScript,
		Workdir:        validation.ResolvedWorkdir,
		CommandJSON:    encodeJSON(validation.CommandPreview, "[]"),
		ValidationJSON: encodeJSON(validation, "{}"),
		StartedAt:      startedAt,
		CreatedAt:      startedAt,
	}
	_ = h.db.SaveInstallRun(run)

	go h.runInstallApply(cfg, validation, runID)

	respond(w, http.StatusAccepted, map[string]any{
		"status":     "started",
		"validation": validation,
		"run":        installRunToPayload(run),
	})
}

func (h *Handler) runInstallApply(cfg *db.UIConfig, validation installValidationResult, runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout())
	defer cancel()

	h.markInstallApplyStarted()
	command := validation.CommandPreview
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	if validation.ResolvedWorkdir != "" {
		cmd.Dir = validation.ResolvedWorkdir
	}
	cmd.Env = append(os.Environ(),
		"VEILKEY_INSTALL_PROFILE="+validation.ResolvedProfile,
		"VEILKEY_INSTALL_ROOT="+validation.ResolvedRoot,
		"VEILKEY_INSTALL_VAULTCENTER_URL="+strings.TrimSpace(cfg.VaultcenterURL),
		"VEILKEY_INSTALL_LOCALVAULT_URL="+strings.TrimSpace(cfg.LocalvaultURL),
		"VEILKEY_VAULTCENTER_URL="+strings.TrimSpace(cfg.VaultcenterURL),
		"VEILKEY_LOCALVAULT_URL="+strings.TrimSpace(cfg.LocalvaultURL),
		"VEILKEY_TLS_CERT="+strings.TrimSpace(cfg.TLSCertPath),
		"VEILKEY_TLS_KEY="+strings.TrimSpace(cfg.TLSKeyPath),
		"VEILKEY_TLS_CA="+strings.TrimSpace(cfg.TLSCAPath),
	)
	output, cmdErr := cmd.CombinedOutput()
	outputTail := trimCommandOutput(output)
	var runErr error = cmdErr
	client := installHTTPClient(cfg.TLSCAPath)
	if runErr == nil {
		if err := checkHealthEndpoint(client, cfg.VaultcenterURL); err != nil {
			runErr = err
		}
	}
	if runErr == nil && strings.TrimSpace(cfg.LocalvaultURL) != "" {
		if err := checkHealthEndpoint(client, cfg.LocalvaultURL); err != nil {
			runErr = err
		}
	}

	finishedAt := time.Now().UTC()
	next := installApplyState{
		Status:         "succeeded",
		LastStartedAt:  h.snapshotState().LastStartedAt,
		LastFinishedAt: &finishedAt,
		LastProfile:    validation.ResolvedProfile,
		LastRoot:       validation.ResolvedRoot,
		LastScript:     validation.ResolvedScript,
		LastCommand:    append([]string(nil), validation.CommandPreview...),
		LastOutput:     outputTail,
		LastRunID:      runID,
	}
	run, loadErr := h.db.GetInstallRun(runID)
	if loadErr == nil && run != nil {
		run.Status = "succeeded"
		run.OutputTail = outputTail
		run.FinishedAt = &finishedAt
	}

	if runErr != nil {
		next.Status = "failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			next.LastError = "install apply command timed out"
		} else {
			next.LastError = runErr.Error()
		}
		if loadErr == nil && run != nil {
			run.Status = "failed"
			run.LastError = next.LastError
			run.OutputTail = outputTail
			run.FinishedAt = &finishedAt
			_ = h.db.SaveInstallRun(run)
		}
		h.setState(next)
		return
	}

	_ = h.markInstallApplyCompleted()
	if loadErr == nil && run != nil {
		_ = h.db.SaveInstallRun(run)
	}
	h.setState(next)
}

func (h *Handler) markInstallApplyStarted() {
	session, err := h.db.GetLatestInstallSession()
	if err != nil || session == nil {
		return
	}
	if strings.TrimSpace(session.LastStage) == "" || strings.EqualFold(strings.TrimSpace(session.LastStage), "language") {
		session.LastStage = "apply_started"
		_ = h.db.SaveInstallSession(session)
	}
}

func (h *Handler) markInstallApplyCompleted() error {
	session, err := h.db.GetLatestInstallSession()
	if err != nil || session == nil {
		return err
	}
	planned := httputil.DecodeStringList(session.PlannedStagesJSON)
	completed := httputil.DecodeStringList(session.CompletedStagesJSON)
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
	session.CompletedStagesJSON = httputil.EncodeStringList(completed)
	session.LastStage = "final_smoke"
	return h.db.SaveInstallSession(session)
}
