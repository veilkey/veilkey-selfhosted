package bulk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	vcrypto "github.com/veilkey/veilkey-go-package/crypto"
)

// ---------------------------------------------------------------------------
// Workflow types
// ---------------------------------------------------------------------------

type bulkApplyRenderedStep struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	TargetPath string `json:"target_path"`
	Content    string `json:"content"`
	Hook       string `json:"hook,omitempty"`
}

type bulkApplyWorkflowStepSummary struct {
	Name        string   `json:"name"`
	Format      string   `json:"format"`
	TargetPath  string   `json:"target_path"`
	Hook        string   `json:"hook,omitempty"`
	Prechecks   []string `json:"prechecks"`
	Postchecks  []string `json:"postchecks"`
	Description string   `json:"description,omitempty"`
}

type bulkApplyWorkflowSummary struct {
	Name              string                         `json:"name"`
	Label             string                         `json:"label"`
	Hook              string                         `json:"hook,omitempty"`
	StepCount         int                            `json:"step_count"`
	Steps             []bulkApplyWorkflowStepSummary `json:"steps"`
	Description       string                         `json:"description,omitempty"`
	Runs              []bulkApplyRunSummary          `json:"runs,omitempty"`
	ValidationStatus  string                         `json:"validation_status,omitempty"`
	ValidationMessage string                         `json:"validation_message,omitempty"`
}

type bulkApplyWorkflowPayload struct {
	Name  string                  `json:"name"`
	Steps []bulkApplyRenderedStep `json:"steps"`
}

type bulkApplyRunSummary struct {
	RunID            string         `json:"run_id"`
	VaultRuntimeHash string         `json:"vault_runtime_hash"`
	WorkflowName     string         `json:"workflow_name"`
	RunKind          string         `json:"run_kind"`
	Status           string         `json:"status"`
	Summary          map[string]any `json:"summary"`
	CreatedAt        string         `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Workflow helpers
// ---------------------------------------------------------------------------

func bulkApplyWorkflowStepSummaryFromTemplate(step bulkApplyTemplateRecord) bulkApplyWorkflowStepSummary {
	prechecks := []string{"ensure_target_parent_exists", "ensure_target_parent_writable"}
	postchecks := []string{}
	switch strings.TrimSpace(step.Format) {
	case "json":
		postchecks = append(postchecks, "json_parse")
	case "json_merge":
		postchecks = append(postchecks, "json_parse", "json_merge_verify")
	case "line_patch":
		postchecks = append(postchecks, "line_patch_verify")
	default:
		postchecks = append(postchecks, "file_written")
	}
	return bulkApplyWorkflowStepSummary{
		Name:       step.Name,
		Format:     step.Format,
		TargetPath: step.TargetPath,
		Hook:       step.Hook,
		Prechecks:  prechecks,
		Postchecks: postchecks,
	}
}

// renderResolvedBulkApplyBody renders the template body with all placeholders
// resolved via h.deps.ResolveTemplateValue. vaultHash scopes the resolution.
func (h *Handler) renderResolvedBulkApplyBody(vaultHash, body string) (string, error) {
	var firstErr error
	rendered := bulkApplyPlaceholderPattern.ReplaceAllStringFunc(body, func(token string) string {
		match := bulkApplyPlaceholderPattern.FindStringSubmatch(token)
		if len(match) != 3 {
			return token
		}
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])

		depKind := "secret"
		if kind == db.RefFamilyVE {
			depKind = "config"
		}

		value, ok := h.deps.ResolveTemplateValue(vaultHash, depKind, name)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to resolve %s.%s", kind, name)
			}
			return token
		}
		return value
	})
	if firstErr != nil {
		return "", firstErr
	}
	return rendered, nil
}

func (h *Handler) buildBulkApplyWorkflowSummary(vaultHash, workflowName string) (*bulkApplyWorkflowSummary, error) {
	_ = h.migrateBulkApplyTemplatesFromDB(vaultHash)
	workflow, validationStatus, validationMessage, err := h.loadBulkApplyWorkflowFile(vaultHash, workflowName)
	if err != nil && validationStatus == "broken" {
		return nil, fmt.Errorf("workflow %s not found", strings.TrimSpace(workflowName))
	}
	summary := &bulkApplyWorkflowSummary{
		Name:              strings.TrimSpace(workflowName),
		ValidationStatus:  validationStatus,
		ValidationMessage: validationMessage,
	}
	if workflow != nil {
		summary.Label = strings.TrimSpace(workflow.Label)
		summary.Description = strings.TrimSpace(workflow.Description)
		summary.StepCount = len(workflow.Steps)
		summary.Steps = make([]bulkApplyWorkflowStepSummary, 0, len(workflow.Steps))
		if len(workflow.Hooks) > 0 {
			summary.Hook = workflow.Hooks[len(workflow.Hooks)-1]
		}
		for _, stepFile := range workflow.Steps {
			tmpl, err := h.loadBulkApplyTemplateRecord(vaultHash, stepFile.Template)
			if err != nil {
				continue
			}
			item := bulkApplyWorkflowStepSummaryFromTemplate(*tmpl)
			summary.Steps = append(summary.Steps, item)
			if item.Hook != "" {
				summary.Hook = item.Hook
			}
		}
	}
	if summary.Label == "" {
		summary.Label = strings.TrimSpace(workflowName)
	}
	return summary, nil
}

func (h *Handler) buildBulkApplyWorkflowPayload(vaultHash, workflowName string) (*bulkApplyWorkflowPayload, error) {
	// Confirm that a live agent is reachable for this vault before rendering.
	if _, err := h.deps.FindAgentURL(vaultHash); err != nil {
		return nil, err
	}
	summary, err := h.buildBulkApplyWorkflowSummary(vaultHash, workflowName)
	if err != nil {
		return nil, err
	}
	payload := &bulkApplyWorkflowPayload{
		Name:  strings.TrimSpace(workflowName),
		Steps: make([]bulkApplyRenderedStep, 0, len(summary.Steps)),
	}
	for _, step := range summary.Steps {
		tmpl, err := h.loadBulkApplyTemplateRecord(vaultHash, step.Name)
		if err != nil {
			return nil, err
		}
		if tmpl.ValidationStatus != "valid" {
			return nil, fmt.Errorf("template %s is broken", step.Name)
		}
		rendered, err := h.renderResolvedBulkApplyBody(vaultHash, tmpl.Body)
		if err != nil {
			return nil, err
		}
		payload.Steps = append(payload.Steps, bulkApplyRenderedStep{
			Name:       step.Name,
			Format:     step.Format,
			TargetPath: step.TargetPath,
			Content:    rendered,
			Hook:       step.Hook,
		})
	}
	return payload, nil
}

func decodeBulkApplyRunSummary(run *db.BulkApplyRun) bulkApplyRunSummary {
	summary := map[string]any{}
	if run != nil && strings.TrimSpace(run.SummaryJSON) != "" {
		_ = json.Unmarshal([]byte(run.SummaryJSON), &summary)
	}
	createdAt := ""
	if run != nil && !run.CreatedAt.IsZero() {
		createdAt = run.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return bulkApplyRunSummary{
		RunID:            run.RunID,
		VaultRuntimeHash: run.VaultRuntimeHash,
		WorkflowName:     run.WorkflowName,
		RunKind:          run.RunKind,
		Status:           run.Status,
		Summary:          summary,
		CreatedAt:        createdAt,
	}
}

func (h *Handler) saveBulkApplyRun(vaultHash, workflowName, runKind, status string, body []byte) {
	summaryJSON := "{}"
	if len(body) > 0 && json.Valid(body) {
		summaryJSON = string(body)
	}
	_ = h.deps.DB().SaveBulkApplyRun(&db.BulkApplyRun{
		RunID:            vcrypto.GenerateUUID(),
		VaultRuntimeHash: strings.TrimSpace(vaultHash),
		WorkflowName:     strings.TrimSpace(workflowName),
		RunKind:          strings.TrimSpace(runKind),
		Status:           strings.TrimSpace(status),
		SummaryJSON:      summaryJSON,
	})
}

// proxyBulkApplyWorkflow builds the rendered payload for a workflow and POSTs
// it to the target agent at path. It uses h.deps.HTTPClient() for transport.
func (h *Handler) proxyBulkApplyWorkflow(r *http.Request, vaultHash, workflowName, path string) (int, []byte, error) {
	agentURL, err := h.deps.FindAgentURL(vaultHash)
	if err != nil {
		return http.StatusNotFound, nil, err
	}
	payload, err := h.buildBulkApplyWorkflowPayload(vaultHash, workflowName)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, agentURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", httputil.ContentTypeJSON)
	resp, err := h.deps.HTTPClient().Do(req)
	if err != nil {
		return http.StatusBadGateway, nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func (h *Handler) proxyAndRecordWorkflow(w http.ResponseWriter, r *http.Request, endpoint, runType, failStatus string) {
	vaultHash := httputil.PathVal(r, "vault")
	workflowName := httputil.PathVal(r, "name")
	if vaultHash == "" || workflowName == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault and workflow are required")
		return
	}
	statusCode, body, err := h.proxyBulkApplyWorkflow(r, vaultHash, workflowName, endpoint)
	if err != nil {
		httputil.RespondError(w, statusCode, err.Error())
		return
	}
	var payload map[string]any
	status := failStatus
	if json.Unmarshal(body, &payload) == nil {
		if raw := strings.TrimSpace(fmt.Sprint(payload["status"])); raw != "" {
			status = raw
		}
	}
	h.saveBulkApplyRun(vaultHash, workflowName, runType, status, body)
	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleBulkApplyWorkflows(w http.ResponseWriter, r *http.Request) {
	vaultHash := httputil.PathVal(r, "vault")
	if vaultHash == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault is required")
		return
	}
	workflows, err := h.listBulkApplyWorkflowSummaries(vaultHash)
	if err != nil {
		httputil.RespondJSON(w, http.StatusOK, map[string]any{
			"vault_runtime_hash": vaultHash,
			"count":              0,
			"workflows":          []bulkApplyWorkflowSummary{},
		})
		return
	}
	httputil.RespondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": vaultHash,
		"count":              len(workflows),
		"workflows":          workflows,
	})
}

func (h *Handler) handleBulkApplyWorkflow(w http.ResponseWriter, r *http.Request) {
	vaultHash := httputil.PathVal(r, "vault")
	workflowName := httputil.PathVal(r, "name")
	if vaultHash == "" || workflowName == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault and workflow are required")
		return
	}
	summary, err := h.buildBulkApplyWorkflowSummary(vaultHash, workflowName)
	if err != nil {
		httputil.RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	rows, err := h.deps.DB().ListBulkApplyRuns(vaultHash, workflowName, 10)
	if err == nil {
		summary.Runs = make([]bulkApplyRunSummary, 0, len(rows))
		for i := range rows {
			summary.Runs = append(summary.Runs, decodeBulkApplyRunSummary(&rows[i]))
		}
	}
	httputil.RespondJSON(w, http.StatusOK, summary)
}

func (h *Handler) handleBulkApplyWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	vaultHash := httputil.PathVal(r, "vault")
	workflowName := httputil.PathVal(r, "name")
	if vaultHash == "" || workflowName == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault and workflow are required")
		return
	}
	rows, err := h.deps.DB().ListBulkApplyRuns(vaultHash, workflowName, 10)
	if err != nil {
		httputil.RespondError(w, http.StatusInternalServerError, "failed to list bulk apply runs")
		return
	}
	runs := make([]bulkApplyRunSummary, 0, len(rows))
	for i := range rows {
		runs = append(runs, decodeBulkApplyRunSummary(&rows[i]))
	}
	httputil.RespondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": vaultHash,
		"workflow_name":      workflowName,
		"count":              len(runs),
		"runs":               runs,
	})
}

func (h *Handler) handleBulkApplyRun(w http.ResponseWriter, r *http.Request) {
	runID := httputil.PathVal(r, "run")
	if runID == "" {
		httputil.RespondError(w, http.StatusBadRequest, "run is required")
		return
	}
	run, err := h.deps.DB().GetBulkApplyRun(runID)
	if err != nil {
		httputil.RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	httputil.RespondJSON(w, http.StatusOK, decodeBulkApplyRunSummary(run))
}

func (h *Handler) handleBulkApplyWorkflowPrecheck(w http.ResponseWriter, r *http.Request) {
	h.proxyAndRecordWorkflow(w, r, "/api/bulk-apply/precheck", "precheck", "precheck_failed")
}

func (h *Handler) handleBulkApplyWorkflowRun(w http.ResponseWriter, r *http.Request) {
	h.proxyAndRecordWorkflow(w, r, "/api/bulk-apply/execute", "run", "apply_failed")
}
