package bulk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/veilkey/veilkey-go-package/httputil"
)

type bulkApplyStep struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	TargetPath string `json:"target_path"`
	Content    string `json:"content"`
	Hook       string `json:"hook"`
}

type bulkApplyWorkflowRequest struct {
	Name  string          `json:"name"`
	Steps []bulkApplyStep `json:"steps"`
}

// defaultBulkApplyTargets are always allowed.
var defaultBulkApplyTargets = map[string]struct{}{
	"/opt/mattermost/config/config.json":                     {},
	"/opt/mattermost/.env":                                   {},
	"/etc/systemd/system/mattermost.service.d/override.conf": {},
	"/etc/gitlab/gitlab.rb":                                  {},
}

// extraBulkApplyTargets is populated from VEILKEY_BULK_APPLY_ALLOWED_PATHS env var
// (comma-separated absolute paths) at init time.
var extraBulkApplyTargets = map[string]struct{}{}

// defaultBulkApplyHooks are always allowed.
var defaultBulkApplyHooks = map[string][]string{
	"reload_systemd":     {"systemctl", "daemon-reload"},
	"restart_mattermost": {"systemctl", "restart", "mattermost"},
	"reconfigure_gitlab": {"gitlab-ctl", "reconfigure"},
}

// extraBulkApplyHooks is populated from VEILKEY_BULK_APPLY_ALLOWED_HOOKS env var
// (comma-separated name:cmd pairs, e.g. "restart_nginx:systemctl restart nginx")
var extraBulkApplyHooks = map[string][]string{}

func init() {
	// Load extra allowed paths from env
	if paths := os.Getenv("VEILKEY_BULK_APPLY_ALLOWED_PATHS"); paths != "" {
		for _, p := range strings.Split(paths, ",") {
			p = strings.TrimSpace(p)
			if p == "" || !filepath.IsAbs(p) || strings.Contains(p, "..") {
				continue
			}
			extraBulkApplyTargets[p] = struct{}{}
		}
	}
	// Load extra allowed hooks from env
	if hooks := os.Getenv("VEILKEY_BULK_APPLY_ALLOWED_HOOKS"); hooks != "" {
		for _, entry := range strings.Split(hooks, ",") {
			entry = strings.TrimSpace(entry)
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				continue
			}
			name := strings.TrimSpace(parts[0])
			cmdParts := strings.Fields(strings.TrimSpace(parts[1]))
			if len(cmdParts) > 0 {
				extraBulkApplyHooks[name] = cmdParts
			}
		}
	}
}

func isAllowedBulkApplyTarget(path string) bool {
	p := strings.TrimSpace(path)
	if _, ok := defaultBulkApplyTargets[p]; ok {
		return true
	}
	if _, ok := extraBulkApplyTargets[p]; ok {
		return true
	}
	return false
}

func getAllowedBulkApplyHook(name string) ([]string, bool) {
	n := strings.TrimSpace(name)
	if cmd, ok := defaultBulkApplyHooks[n]; ok {
		return cmd, true
	}
	if cmd, ok := extraBulkApplyHooks[n]; ok {
		return cmd, true
	}
	return nil, false
}

func recursiveJSONMerge(dst map[string]any, src map[string]any) map[string]any {
	for key, srcValue := range src {
		srcMap, srcIsMap := srcValue.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap {
			dst[key] = recursiveJSONMerge(dstMap, srcMap)
			continue
		}
		dst[key] = srcValue
	}
	return dst
}

func writeAtomically(path string, content []byte) error {
	dir := filepath.Dir(path)
	var (
		mode     os.FileMode = 0644
		uid              = -1
		gid              = -1
		haveStat bool
	)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		haveStat = true
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid = int(stat.Uid)
			gid = int(stat.Gid)
		}
	}
	tmp, err := os.CreateTemp(dir, ".bulk-apply-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(content); err != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			return fmt.Errorf("write failed: %w; close also failed: %v", err, closeErr)
		}
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if haveStat && uid >= 0 && gid >= 0 {
		if err := os.Chown(tmpName, uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmpName, path)
}

func validateBulkApplyStep(step bulkApplyStep) error {
	if strings.TrimSpace(step.Name) == "" {
		return fmt.Errorf("step name is required")
	}
	if !isAllowedBulkApplyTarget(step.TargetPath) {
		return fmt.Errorf("target path is not allowed: %s", step.TargetPath)
	}
	switch strings.TrimSpace(step.Format) {
	case "env", "json", "json_merge", "raw":
	default:
		return fmt.Errorf("unsupported format: %s", step.Format)
	}
	if strings.TrimSpace(step.Content) == "" {
		return fmt.Errorf("content is required")
	}
	parent := filepath.Dir(step.TargetPath)
	if _, err := os.Stat(parent); err != nil {
		return fmt.Errorf("parent path not found: %s", parent)
	}
	switch strings.TrimSpace(step.Format) {
	case "json", "json_merge":
		var payload map[string]any
		if err := json.Unmarshal([]byte(step.Content), &payload); err != nil {
			return fmt.Errorf("invalid json content: %w", err)
		}
	}
	if hook := strings.TrimSpace(step.Hook); hook != "" {
		if _, ok := getAllowedBulkApplyHook(hook); !ok {
			return fmt.Errorf("hook is not allowed: %s", hook)
		}
	}
	return nil
}

func (h *Handler) handleBulkApplyPrecheck(w http.ResponseWriter, r *http.Request) {
	var req bulkApplyWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Steps) == 0 {
		respondError(w, 400, "workflow name and steps are required")
		return
	}
	if len(req.Steps) > httputil.MaxBulkItems {
		respondError(w, 400, "too many steps (max 200)")
		return
	}
	checks := make([]map[string]any, 0, len(req.Steps))
	for _, step := range req.Steps {
		err := validateBulkApplyStep(step)
		status := "ok"
		message := "ready"
		if err != nil {
			status = "failed"
			message = err.Error()
		}
		checks = append(checks, map[string]any{
			"step":    step.Name,
			"status":  status,
			"message": message,
		})
		if err != nil {
			respondJSON(w, 409, map[string]any{
				"workflow": req.Name,
				"status":   "precheck_failed",
				"checks":   checks,
			})
			return
		}
	}
	respondJSON(w, 200, map[string]any{
		"workflow": req.Name,
		"status":   "ready",
		"checks":   checks,
	})
}

func applyBulkApplyStep(step bulkApplyStep) error {
	switch strings.TrimSpace(step.Format) {
	case "env", "raw", "json":
		return writeAtomically(step.TargetPath, []byte(step.Content))
	case "json_merge":
		var current map[string]any
		if raw, err := os.ReadFile(step.TargetPath); err == nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, &current); err != nil {
				return fmt.Errorf("failed to parse existing json: %w", err)
			}
		}
		if current == nil {
			current = map[string]any{}
		}
		var patch map[string]any
		if err := json.Unmarshal([]byte(step.Content), &patch); err != nil {
			return fmt.Errorf("invalid merge json: %w", err)
		}
		merged := recursiveJSONMerge(current, patch)
		rendered, err := json.MarshalIndent(merged, "", "    ")
		if err != nil {
			return err
		}
		rendered = append(rendered, '\n')
		return writeAtomically(step.TargetPath, rendered)
	default:
		return fmt.Errorf("unsupported format: %s", step.Format)
	}
}

func runAllowedHook(name string) (string, error) {
	cmdv, ok := getAllowedBulkApplyHook(name)
	if !ok {
		return "", fmt.Errorf("hook is not allowed: %s", name)
	}
	cmd := exec.Command(cmdv[0], cmdv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func orderedHooks(steps []bulkApplyStep) []string {
	seen := map[string]struct{}{}
	hooks := make([]string, 0, len(steps)+1)
	needsSystemdReload := false
	for _, step := range steps {
		if strings.TrimSpace(step.TargetPath) == "/etc/systemd/system/mattermost.service.d/override.conf" {
			needsSystemdReload = true
		}
		hook := strings.TrimSpace(step.Hook)
		if hook == "" {
			continue
		}
		if _, ok := seen[hook]; ok {
			continue
		}
		seen[hook] = struct{}{}
		hooks = append(hooks, hook)
	}
	if needsSystemdReload {
		foundRestart := false
		for _, hook := range hooks {
			if hook == "restart_mattermost" {
				foundRestart = true
				break
			}
		}
		if foundRestart {
			if _, ok := seen["reload_systemd"]; !ok {
				hooks = append([]string{"reload_systemd"}, hooks...)
			} else {
				reordered := []string{"reload_systemd"}
				for _, hook := range hooks {
					if hook != "reload_systemd" {
						reordered = append(reordered, hook)
					}
				}
				hooks = reordered
			}
		}
	}
	return hooks
}

func postchecksForStep(step bulkApplyStep) []string {
	checks := []string{}
	switch strings.TrimSpace(step.Format) {
	case "json":
		checks = append(checks, "json_parse")
	case "json_merge":
		checks = append(checks, "json_parse", "json_merge_verify")
	case "raw", "env":
		checks = append(checks, "file_written")
	default:
		checks = append(checks, "file_written")
	}
	switch strings.TrimSpace(step.TargetPath) {
	case "/opt/mattermost/config/config.json":
		checks = append(checks, "mattermost_config_required_keys")
	case "/etc/systemd/system/mattermost.service.d/override.conf":
		checks = append(checks, "systemd_override_parse")
	}
	return checks
}

func runPostcheck(step bulkApplyStep, name string) (map[string]any, error) {
	result := map[string]any{
		"name":   name,
		"status": "ok",
	}
	switch strings.TrimSpace(name) {
	case "file_written":
		info, err := os.Stat(step.TargetPath)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		result["message"] = "file exists"
		result["size"] = info.Size()
		return result, nil
	case "json_parse":
		raw, err := os.ReadFile(step.TargetPath)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		result["message"] = "json parsed"
		return result, nil
	case "json_merge_verify":
		raw, err := os.ReadFile(step.TargetPath)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		result["message"] = "merged json verified"
		return result, nil
	case "mattermost_config_required_keys":
		raw, err := os.ReadFile(step.TargetPath)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		serviceSettings, _ := payload["ServiceSettings"].(map[string]any)
		sqlSettings, _ := payload["SqlSettings"].(map[string]any)
		if strings.TrimSpace(fmt.Sprint(serviceSettings["SiteURL"])) == "" {
			err := fmt.Errorf("ServiceSettings.SiteURL is required")
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		if strings.TrimSpace(fmt.Sprint(sqlSettings["DataSource"])) == "" {
			err := fmt.Errorf("SqlSettings.DataSource is required")
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		result["message"] = "mattermost required keys verified"
		return result, nil
	case "systemd_override_parse":
		raw, err := os.ReadFile(step.TargetPath)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		text := string(raw)
		if !strings.Contains(text, "[Service]") {
			err := fmt.Errorf("override missing [Service] section")
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		if !strings.Contains(text, "Environment=") && !strings.Contains(text, "EnvironmentFile=") {
			err := fmt.Errorf("override missing Environment or EnvironmentFile entries")
			result["status"] = "failed"
			result["message"] = err.Error()
			return result, err
		}
		result["message"] = "systemd override verified"
		return result, nil
	default:
		result["message"] = "check skipped"
		return result, nil
	}
}

func (h *Handler) handleBulkApplyExecute(w http.ResponseWriter, r *http.Request) {
	var req bulkApplyWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Steps) == 0 {
		respondError(w, 400, "workflow name and steps are required")
		return
	}
	if len(req.Steps) > httputil.MaxBulkItems {
		respondError(w, 400, "too many steps (max 200)")
		return
	}
	results := make([]map[string]any, 0, len(req.Steps))
	postcheckResults := make([]map[string]any, 0, len(req.Steps))
	hookResults := make([]map[string]any, 0)
	hooks := orderedHooks(req.Steps)
	for _, step := range req.Steps {
		if err := validateBulkApplyStep(step); err != nil {
			respondJSON(w, 409, map[string]any{
				"workflow":     req.Name,
				"status":       "precheck_failed",
				"step":         step.Name,
				"error":        err.Error(),
				"results":      results,
				"postchecks":   postcheckResults,
				"hook_results": hookResults,
			})
			return
		}
		if err := applyBulkApplyStep(step); err != nil {
			respondJSON(w, 500, map[string]any{
				"workflow":     req.Name,
				"status":       "apply_failed",
				"step":         step.Name,
				"error":        err.Error(),
				"results":      results,
				"postchecks":   postcheckResults,
				"hook_results": hookResults,
			})
			return
		}
		stepResult := map[string]any{
			"step":       step.Name,
			"status":     "applied",
			"target":     step.TargetPath,
			"postchecks": []map[string]any{},
		}
		checkRows := make([]map[string]any, 0, len(postchecksForStep(step)))
		for _, checkName := range postchecksForStep(step) {
			checkRow, err := runPostcheck(step, checkName)
			checkRows = append(checkRows, checkRow)
			if err != nil {
				stepResult["status"] = "postcheck_failed"
				stepResult["postchecks"] = checkRows
				results = append(results, stepResult)
				postcheckResults = append(postcheckResults, map[string]any{
					"step":   step.Name,
					"checks": checkRows,
				})
				respondJSON(w, 500, map[string]any{
					"workflow":     req.Name,
					"status":       "postcheck_failed",
					"step":         step.Name,
					"error":        err.Error(),
					"results":      results,
					"postchecks":   postcheckResults,
					"hook_results": hookResults,
				})
				return
			}
		}
		stepResult["postchecks"] = checkRows
		results = append(results, stepResult)
		postcheckResults = append(postcheckResults, map[string]any{
			"step":   step.Name,
			"checks": checkRows,
		})
	}
	for _, hook := range hooks {
		output, err := runAllowedHook(hook)
		hookRow := map[string]any{
			"name":   hook,
			"status": "ok",
		}
		if strings.TrimSpace(output) != "" {
			hookRow["output"] = output
		}
		if err != nil {
			hookRow["status"] = "failed"
			hookRow["message"] = err.Error()
			hookResults = append(hookResults, hookRow)
			respondJSON(w, 500, map[string]any{
				"workflow":     req.Name,
				"status":       "hook_failed",
				"hook":         hook,
				"error":        err.Error(),
				"results":      results,
				"postchecks":   postcheckResults,
				"hook_results": hookResults,
			})
			return
		}
		hookResults = append(hookResults, hookRow)
	}
	respondJSON(w, 200, map[string]any{
		"workflow":     req.Name,
		"status":       "applied",
		"results":      results,
		"postchecks":   postcheckResults,
		"hooks":        hooks,
		"hook_results": hookResults,
	})
}
