package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"veilkey-keycenter/internal/crypto"
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
	targetType := strings.TrimSpace(cfg.TargetType)
	profile := strings.TrimSpace(cfg.InstallProfile)
	if targetType == "lxc-allinone" && profile == "" {
		return "proxmox-lxc-allinone"
	}
	switch profile {
	case "", "linux-host":
		if strings.TrimSpace(cfg.LocalvaultURL) != "" {
			return "proxmox-host-localvault"
		}
		return "proxmox-host"
	case "lxc-allinone", "all-in-one", "linux-all-in-one":
		return "proxmox-lxc-allinone"
	default:
		return profile
	}
}

func installTargetLabel(cfg *db.UIConfig) string {
	if strings.TrimSpace(cfg.TargetType) != "" {
		return strings.TrimSpace(cfg.TargetType)
	}
	switch resolveInstallProfile(cfg) {
	case "proxmox-lxc-allinone":
		return "lxc-allinone"
	case "proxmox-host-localvault":
		return "host-localvault"
	default:
		if strings.TrimSpace(cfg.LocalvaultURL) != "" {
			return "host-existing-localvault"
		}
		return "linux-host"
	}
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

	target := installTargetLabel(cfg)
	if target == "lxc-allinone" {
		result.ResolvedRoot = "/"
	}
	if target == "linux-host" {
		result.Warnings = append(result.Warnings, "linux-host quick path is not yet a validated production install target; prefer lxc-allinone or host-localvault")
	}
	if target == "lxc-allinone" {
		if strings.TrimSpace(cfg.TargetMode) == "" {
			result.Valid = false
			result.Errors = append(result.Errors, "lxc-allinone requires target_mode=new or existing")
		}
		if strings.TrimSpace(cfg.TargetVMID) == "" {
			result.Valid = false
			result.Errors = append(result.Errors, "lxc-allinone requires target_vmid")
		}
		if strings.TrimSpace(cfg.TargetNode) == "" {
			result.Warnings = append(result.Warnings, "target_node is empty; proxmox node selection should be explicit")
		}
		for _, passwordPath := range []string{
			proxmoxLXCPasswordFile("VEILKEY_INSTALL_KEYCENTER_PASSWORD_FILE", "/etc/veilkey/keycenter.password"),
			proxmoxLXCPasswordFile("VEILKEY_INSTALL_LOCALVAULT_PASSWORD_FILE", "/etc/veilkey/localvault.password"),
		} {
			if _, err := os.Stat(passwordPath); err != nil {
				result.Valid = false
				result.Errors = append(result.Errors, "required password file is not available: "+passwordPath)
			}
		}
		if strings.TrimSpace(cfg.TargetMode) == "new" {
			if proxmoxLXCTemplateVMID() == "" {
				result.Valid = false
				result.Errors = append(result.Errors, "new lxc provisioning requires VEILKEY_PROXMOX_LXC_TEMPLATE_VMID")
			}
			if proxmoxLXCNet0Template() == "" {
				result.Valid = false
				result.Errors = append(result.Errors, "new lxc provisioning requires VEILKEY_PROXMOX_LXC_NET0_TEMPLATE")
			}
		}
		if strings.TrimSpace(cfg.KeycenterURL) != "" {
			result.Warnings = append(result.Warnings, "keycenter_url is set before provisioning; prefer public_base_url preview until the target LXC exists")
		}
	}
	if target == "host-localvault" && strings.TrimSpace(cfg.KeycenterURL) == "" {
		result.Warnings = append(result.Warnings, "host-localvault install usually expects keycenter_url to be set before activation")
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
	if target == "lxc-allinone" {
		result.DangerousRoot = false
		result.NeedsConfirmation = false
	}
	if result.DangerousRoot {
		result.Warnings = append(result.Warnings, "install_root targets the live filesystem root")
		if !req.ConfirmDangerousRoot {
			result.Valid = false
			result.NeedsConfirmation = true
			result.Errors = append(result.Errors, "dangerous install_root requires explicit confirmation")
		}
	}

	if strings.TrimSpace(cfg.KeycenterURL) == "" {
		result.Warnings = append(result.Warnings, "keycenter_url is empty; post-install verification may be limited")
	}
	if strings.TrimSpace(cfg.LocalvaultURL) == "" {
		result.Warnings = append(result.Warnings, "localvault_url is empty; localvault health verification will be skipped")
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
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func proxmoxLXCPasswordFile(envName, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value
	}
	return fallback
}

func proxmoxLXCTemplateVMID() string {
	return strings.TrimSpace(os.Getenv("VEILKEY_PROXMOX_LXC_TEMPLATE_VMID"))
}

func proxmoxLXCNet0Template() string {
	return strings.TrimSpace(os.Getenv("VEILKEY_PROXMOX_LXC_NET0_TEMPLATE"))
}

func proxmoxHostRoot() string {
	if value := strings.TrimSpace(os.Getenv("VEILKEY_PROXMOX_HOST_ROOT")); value != "" {
		return value
	}
	return "/"
}

func proxmoxLXCAPIBase(workdir string) string {
	if value := strings.TrimSpace(os.Getenv("VEILKEY_INSTALLER_GITLAB_API_BASE")); value != "" {
		return value
	}
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(out))
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.TrimPrefix(raw, "https://")
	if idx := strings.Index(raw, "/"); idx > 0 {
		return "https://" + raw[:idx] + "/api/v4"
	}
	return ""
}

func proxmoxLXCGitLabHost(apiBase string) string {
	apiBase = strings.TrimSpace(apiBase)
	apiBase = strings.TrimPrefix(apiBase, "https://")
	apiBase = strings.TrimPrefix(apiBase, "http://")
	if idx := strings.Index(apiBase, "/"); idx > 0 {
		return apiBase[:idx]
	}
	return ""
}

func installShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func runCommand(ctx context.Context, output *strings.Builder, name string, args ...string) error {
	output.WriteString("$ " + name)
	for _, arg := range args {
		output.WriteString(" " + installShellQuote(arg))
	}
	output.WriteString("\n")
	cmd := exec.CommandContext(ctx, name, args...)
	data, err := cmd.CombinedOutput()
	if len(data) > 0 {
		output.Write(data)
		if data[len(data)-1] != '\n' {
			output.WriteByte('\n')
		}
	}
	return err
}

func appendHostCompanionFailureGuidance(output *strings.Builder, workdir, bundleRoot string, err error) error {
	guidance := "LXC install succeeded but host companion failed — run proxmox-host-cli-install.sh manually"
	if output != nil {
		output.WriteString(guidance)
		if bundleRoot != "" {
			output.WriteString(" with bundle ")
			output.WriteString(bundleRoot)
		}
		output.WriteString("\n")
		output.WriteString("Suggested command: ")
		output.WriteString(filepath.Join(workdir, "scripts", "proxmox-host-cli-install.sh"))
		output.WriteString(" ")
		output.WriteString(proxmoxHostRoot())
		if bundleRoot != "" {
			output.WriteString(" ")
			output.WriteString(bundleRoot)
		}
		output.WriteString("\n")
	}
	if err == nil {
		return errors.New(guidance)
	}
	return fmt.Errorf("%s: %w", guidance, err)
}

func runProxmoxLXCInstall(ctx context.Context, cfg *db.UIConfig, validation installValidationResult, runID string) (string, error) {
	vmid := strings.TrimSpace(cfg.TargetVMID)
	if vmid == "" {
		return "", fmt.Errorf("target_vmid is required")
	}
	if _, err := strconv.Atoi(vmid); err != nil {
		return "", fmt.Errorf("target_vmid must be numeric")
	}

	workdir := validation.ResolvedWorkdir
	if workdir == "" {
		workdir = installWorkdir(cfg)
	}
	if workdir == "" {
		return "", fmt.Errorf("install workdir is not configured")
	}

	keycenterPasswordFile := proxmoxLXCPasswordFile("VEILKEY_INSTALL_KEYCENTER_PASSWORD_FILE", "/etc/veilkey/keycenter.password")
	localvaultPasswordFile := proxmoxLXCPasswordFile("VEILKEY_INSTALL_LOCALVAULT_PASSWORD_FILE", "/etc/veilkey/localvault.password")
	keycenterPassword, err := os.ReadFile(keycenterPasswordFile)
	if err != nil {
		return "", fmt.Errorf("failed to read keycenter password file: %w", err)
	}
	localvaultPassword, err := os.ReadFile(localvaultPasswordFile)
	if err != nil {
		return "", fmt.Errorf("failed to read localvault password file: %w", err)
	}

	tmpRoot := filepath.Join(os.TempDir(), "veilkey-install-runs", runID)
	if err := os.MkdirAll(tmpRoot, 0700); err != nil {
		return "", err
	}
	installerArchive := filepath.Join(tmpRoot, "installer.tgz")
	bundleRoot := filepath.Join(tmpRoot, "bundle")
	bundleArchive := filepath.Join(tmpRoot, "bundle.tgz")
	hostBundleRoot := filepath.Join(tmpRoot, "host-cli-bundle")
	passwordFile := filepath.Join(tmpRoot, "password")

	installerRoot := filepath.Clean(filepath.Join(workdir, ".."))
	if err := os.WriteFile(passwordFile, []byte("VEILKEY_KEYCENTER_PASSWORD="+strings.TrimSpace(string(keycenterPassword))+"\nVEILKEY_LOCALVAULT_PASSWORD="+strings.TrimSpace(string(localvaultPassword))+"\n"), 0600); err != nil {
		return "", err
	}

	apiBase := proxmoxLXCAPIBase(workdir)
	bundleCmd := exec.CommandContext(ctx, filepath.Join(workdir, "install.sh"), "bundle", validation.ResolvedProfile, bundleRoot)
	bundleCmd.Dir = workdir
	bundleCmd.Env = append(os.Environ(),
		"VEILKEY_INSTALLER_GITLAB_API_BASE="+apiBase,
		"VEILKEY_GITLAB_HOST="+proxmoxLXCGitLabHost(apiBase),
	)
	bundleOut, err := bundleCmd.CombinedOutput()
	logOutput := strings.Builder{}
	logOutput.WriteString("$ " + filepath.Join(workdir, "install.sh") + " bundle " + validation.ResolvedProfile + " " + bundleRoot + "\n")
	logOutput.Write(bundleOut)
	if err != nil {
		return logOutput.String(), err
	}

	if err := runCommand(ctx, &logOutput, "tar", "-C", installerRoot, "-czf", installerArchive, "installer"); err != nil {
		return logOutput.String(), err
	}
	if err := runCommand(ctx, &logOutput, "tar", "-C", tmpRoot, "-czf", bundleArchive, "bundle"); err != nil {
		return logOutput.String(), err
	}
	if strings.TrimSpace(cfg.TargetType) == "lxc-allinone" && strings.TrimSpace(cfg.TargetMode) == "new" {
		templateVMID := proxmoxLXCTemplateVMID()
		net0 := proxmoxLXCNet0Template()
		if templateVMID == "" || net0 == "" {
			return logOutput.String(), fmt.Errorf("new lxc provisioning requires VEILKEY_PROXMOX_LXC_TEMPLATE_VMID and VEILKEY_PROXMOX_LXC_NET0_TEMPLATE")
		}
		net0 = strings.ReplaceAll(net0, "%VMID%", vmid)
		if err := runCommand(ctx, &logOutput, "pct", "clone", templateVMID, vmid, "--hostname", "veilkey-install-"+vmid, "--full", "1"); err != nil {
			return logOutput.String(), err
		}
		if err := runCommand(ctx, &logOutput, "pct", "set", vmid, "-net0", net0); err != nil {
			return logOutput.String(), err
		}
	}

	if err := runCommand(ctx, &logOutput, "pct", "start", vmid); err != nil && !strings.Contains(logOutput.String(), "already running") {
		return logOutput.String(), err
	}
	if err := runCommand(ctx, &logOutput, "pct", "push", vmid, installerArchive, "/root/veilkey-installer.tgz"); err != nil {
		return logOutput.String(), err
	}
	if err := runCommand(ctx, &logOutput, "pct", "push", vmid, bundleArchive, "/root/veilkey-bundle.tgz"); err != nil {
		return logOutput.String(), err
	}
	if err := runCommand(ctx, &logOutput, "pct", "push", vmid, passwordFile, "/root/veilkey-password"); err != nil {
		return logOutput.String(), err
	}
	if err := runCommand(ctx, &logOutput, "pct", "exec", vmid, "--", "bash", "-lc", "mkdir -p /opt/veilkey/data /root/veilkey-installer /root/veilkey-bundle && mv /root/veilkey-password /opt/veilkey/data/password && tar -xzf /root/veilkey-installer.tgz -C /root/veilkey-installer && tar -xzf /root/veilkey-bundle.tgz -C /root/veilkey-bundle && chmod 600 /opt/veilkey/data/password"); err != nil {
		return logOutput.String(), err
	}
	installScript := "cd /root/veilkey-installer/installer && ./scripts/proxmox-lxc-allinone-install.sh --activate / /root/veilkey-bundle/bundle"
	if err := runCommand(ctx, &logOutput, "pct", "exec", vmid, "--", "bash", "-lc", installScript); err != nil {
		return logOutput.String(), err
	}
	if err := runCommand(ctx, &logOutput, "pct", "exec", vmid, "--", "bash", "-lc", "curl -fsSk https://127.0.0.1:10181/health && echo && curl -fsSk https://127.0.0.1:10180/health && echo"); err != nil {
		return logOutput.String(), err
	}
	if cfg.HostCompanion {
		hostBundleCmd := exec.CommandContext(ctx, filepath.Join(workdir, "install.sh"), "bundle", "proxmox-host-cli", hostBundleRoot)
		hostBundleCmd.Dir = workdir
		hostBundleCmd.Env = append(os.Environ(),
			"VEILKEY_INSTALLER_GITLAB_API_BASE="+apiBase,
			"VEILKEY_GITLAB_HOST="+proxmoxLXCGitLabHost(apiBase),
		)
		hostBundleOut, bundleErr := hostBundleCmd.CombinedOutput()
		logOutput.WriteString("$ " + filepath.Join(workdir, "install.sh") + " bundle proxmox-host-cli " + hostBundleRoot + "\n")
		logOutput.Write(hostBundleOut)
		if bundleErr != nil {
			return logOutput.String(), appendHostCompanionFailureGuidance(&logOutput, workdir, hostBundleRoot, bundleErr)
		}
		hostCompanionScript := filepath.Join(workdir, "scripts", "proxmox-host-cli-install.sh")
		hostArgs := []string{hostCompanionScript, proxmoxHostRoot(), hostBundleRoot}
		if err := runCommand(ctx, &logOutput, hostArgs[0], hostArgs[1:]...); err != nil {
			return logOutput.String(), appendHostCompanionFailureGuidance(&logOutput, workdir, hostBundleRoot, err)
		}
	}
	return logOutput.String(), nil
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
		Profile:        resolveInstallProfile(cfg),
		InstallRoot:    strings.TrimSpace(cfg.InstallRoot),
		State:          state,
	}
	s.respondJSON(w, http.StatusOK, payload)
}

func (s *Server) handleGetInstallRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.db.ListInstallRuns(20)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list install runs")
		return
	}
	payload := make([]installRunPayload, 0, len(runs))
	for i := range runs {
		run := runs[i]
		payload = append(payload, installRunToPayload(&run))
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"runs": payload})
}

func (s *Server) handleValidateInstallApply(w http.ResponseWriter, r *http.Request) {
	var req installValidateRequest
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	result := validateInstallConfig(cfg, req)
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
		StartedAt:      time.Now().UTC(),
		CreatedAt:      time.Now().UTC(),
	}
	finishedAt := time.Now().UTC()
	run.FinishedAt = &finishedAt
	if !result.Valid {
		run.Status = "rejected"
		run.LastError = strings.Join(result.Errors, "; ")
	}
	_ = s.db.SaveInstallRun(run)
	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}
	s.respondJSON(w, status, map[string]any{
		"validation": result,
		"run":        installRunToPayload(run),
	})
}

func (s *Server) handleRunInstallApply(w http.ResponseWriter, r *http.Request) {
	var req installValidateRequest
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	validation := validateInstallConfig(cfg, req)
	if !validation.Valid {
		s.respondJSON(w, http.StatusBadRequest, map[string]any{"validation": validation})
		return
	}

	s.installMu.Lock()
	if s.installState.Status == "running" {
		s.installMu.Unlock()
		s.respondError(w, http.StatusConflict, "install apply is already running")
		return
	}
	startedAt := time.Now().UTC()
	runID := crypto.GenerateUUID()
	state := installApplyState{
		Status:        "running",
		LastStartedAt: &startedAt,
		LastProfile:   validation.ResolvedProfile,
		LastRoot:      validation.ResolvedRoot,
		LastScript:    validation.ResolvedScript,
		LastCommand:   append([]string(nil), validation.CommandPreview...),
		LastRunID:     runID,
	}
	s.installState = state
	s.installMu.Unlock()

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
	_ = s.db.SaveInstallRun(run)

	go s.runInstallApply(cfg, validation, runID)

	s.respondJSON(w, http.StatusAccepted, map[string]any{
		"status":     "started",
		"validation": validation,
		"run":        installRunToPayload(run),
	})
}

func (s *Server) runInstallApply(cfg *db.UIConfig, validation installValidationResult, runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout())
	defer cancel()

	s.markInstallApplyStarted()
	var (
		outputTail string
		err        error
	)
	if strings.TrimSpace(cfg.TargetType) == "lxc-allinone" {
		var output string
		output, err = runProxmoxLXCInstall(ctx, cfg, validation, runID)
		outputTail = trimCommandOutput([]byte(output))
	} else {
		command := validation.CommandPreview
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		if validation.ResolvedWorkdir != "" {
			cmd.Dir = validation.ResolvedWorkdir
		}
		cmd.Env = append(os.Environ(),
			"VEILKEY_INSTALL_PROFILE="+validation.ResolvedProfile,
			"VEILKEY_INSTALL_ROOT="+validation.ResolvedRoot,
			"VEILKEY_INSTALL_KEYCENTER_URL="+strings.TrimSpace(cfg.KeycenterURL),
			"VEILKEY_INSTALL_LOCALVAULT_URL="+strings.TrimSpace(cfg.LocalvaultURL),
			"VEILKEY_KEYCENTER_URL="+strings.TrimSpace(cfg.KeycenterURL),
			"VEILKEY_LOCALVAULT_URL="+strings.TrimSpace(cfg.LocalvaultURL),
			"VEILKEY_TLS_CERT="+strings.TrimSpace(cfg.TLSCertPath),
			"VEILKEY_TLS_KEY="+strings.TrimSpace(cfg.TLSKeyPath),
			"VEILKEY_TLS_CA="+strings.TrimSpace(cfg.TLSCAPath),
		)

		output, cmdErr := cmd.CombinedOutput()
		err = cmdErr
		outputTail = trimCommandOutput(output)
		client := installHTTPClient(cfg.TLSCAPath)
		if err == nil {
			if healthErr := checkHealthEndpoint(client, cfg.KeycenterURL); healthErr != nil {
				err = healthErr
			}
		}
		if err == nil && strings.TrimSpace(cfg.LocalvaultURL) != "" {
			if healthErr := checkHealthEndpoint(client, cfg.LocalvaultURL); healthErr != nil {
				err = healthErr
			}
		}
	}

	finishedAt := time.Now().UTC()
	next := installApplyState{
		Status:         "succeeded",
		LastStartedAt:  s.snapshotInstallApply().LastStartedAt,
		LastFinishedAt: &finishedAt,
		LastProfile:    validation.ResolvedProfile,
		LastRoot:       validation.ResolvedRoot,
		LastScript:     validation.ResolvedScript,
		LastCommand:    append([]string(nil), validation.CommandPreview...),
		LastOutput:     outputTail,
		LastRunID:      runID,
	}
	run, loadErr := s.db.GetInstallRun(runID)
	if loadErr == nil && run != nil {
		run.Status = "succeeded"
		run.OutputTail = outputTail
		run.FinishedAt = &finishedAt
	}

	if err != nil {
		next.Status = "failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			next.LastError = "install apply command timed out"
		} else {
			next.LastError = err.Error()
		}
		if loadErr == nil && run != nil {
			run.Status = "failed"
			run.LastError = next.LastError
			run.OutputTail = outputTail
			run.FinishedAt = &finishedAt
			_ = s.db.SaveInstallRun(run)
		}
		s.setInstallApplyState(next)
		return
	}

	_ = s.markInstallApplyCompleted()
	if loadErr == nil && run != nil {
		_ = s.db.SaveInstallRun(run)
	}
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
