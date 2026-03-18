package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	vcrypto "veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

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

func (s *Server) renderResolvedBulkApplyBody(agent *agentInfo, body string) (string, error) {
	if agent == nil {
		return "", fmt.Errorf("agent is required")
	}
	var firstErr error
	rendered := bulkApplyPlaceholderPattern.ReplaceAllStringFunc(body, func(token string) string {
		match := bulkApplyPlaceholderPattern.FindStringSubmatch(token)
		if len(match) != 3 {
			return token
		}
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])
		var (
			value string
			ok    bool
		)
		if kind == "VK" {
			value, ok = s.resolveBulkApplySecretValue(agent, name)
		} else {
			value, ok = s.resolveBulkApplyConfigValue(agent, name)
		}
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

func (s *Server) buildBulkApplyWorkflowSummary(vaultHash, workflowName string) (*bulkApplyWorkflowSummary, error) {
	_ = s.migrateBulkApplyTemplatesFromDB(vaultHash)
	workflow, validationStatus, validationMessage, err := s.loadBulkApplyWorkflowFile(vaultHash, workflowName)
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
			tmpl, err := s.loadBulkApplyTemplateRecord(vaultHash, stepFile.Template)
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

func (s *Server) buildBulkApplyWorkflowPayload(vaultHash, workflowName string) (*bulkApplyWorkflowPayload, error) {
	agent, err := s.findAgent(vaultHash)
	if err != nil {
		return nil, err
	}
	summary, err := s.buildBulkApplyWorkflowSummary(vaultHash, workflowName)
	if err != nil {
		return nil, err
	}
	payload := &bulkApplyWorkflowPayload{
		Name:  strings.TrimSpace(workflowName),
		Steps: make([]bulkApplyRenderedStep, 0, len(summary.Steps)),
	}
	for _, step := range summary.Steps {
		tmpl, err := s.loadBulkApplyTemplateRecord(vaultHash, step.Name)
		if err != nil {
			return nil, err
		}
		if tmpl.ValidationStatus != "valid" {
			return nil, fmt.Errorf("template %s is broken", step.Name)
		}
		rendered, err := s.renderResolvedBulkApplyBody(agent, tmpl.Body)
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

func (s *Server) saveBulkApplyRun(vaultHash, workflowName, runKind, status string, body []byte) {
	summaryJSON := "{}"
	if len(body) > 0 && json.Valid(body) {
		summaryJSON = string(body)
	}
	_ = s.db.SaveBulkApplyRun(&db.BulkApplyRun{
		RunID:            vcrypto.GenerateUUID(),
		VaultRuntimeHash: strings.TrimSpace(vaultHash),
		WorkflowName:     strings.TrimSpace(workflowName),
		RunKind:          strings.TrimSpace(runKind),
		Status:           strings.TrimSpace(status),
		SummaryJSON:      summaryJSON,
	})
}

func (s *Server) handleBulkApplyWorkflows(w http.ResponseWriter, r *http.Request) {
	vaultHash := pathVal(r, "vault")
	if vaultHash == "" {
		s.respondError(w, http.StatusBadRequest, "vault is required")
		return
	}
	workflows, err := s.listBulkApplyWorkflowSummaries(vaultHash)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"vault_runtime_hash": vaultHash,
			"count":              0,
			"workflows":          []bulkApplyWorkflowSummary{},
		})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": vaultHash,
		"count":              len(workflows),
		"workflows":          workflows,
	})
}

func (s *Server) handleBulkApplyWorkflow(w http.ResponseWriter, r *http.Request) {
	vaultHash := pathVal(r, "vault")
	workflowName := pathVal(r, "name")
	if vaultHash == "" || workflowName == "" {
		s.respondError(w, http.StatusBadRequest, "vault and workflow are required")
		return
	}
	summary, err := s.buildBulkApplyWorkflowSummary(vaultHash, workflowName)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	rows, err := s.db.ListBulkApplyRuns(vaultHash, workflowName, 10)
	if err == nil {
		summary.Runs = make([]bulkApplyRunSummary, 0, len(rows))
		for i := range rows {
			summary.Runs = append(summary.Runs, decodeBulkApplyRunSummary(&rows[i]))
		}
	}
	s.respondJSON(w, http.StatusOK, summary)
}

func (s *Server) handleBulkApplyWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	vaultHash := pathVal(r, "vault")
	workflowName := pathVal(r, "name")
	if vaultHash == "" || workflowName == "" {
		s.respondError(w, http.StatusBadRequest, "vault and workflow are required")
		return
	}
	rows, err := s.db.ListBulkApplyRuns(vaultHash, workflowName, 10)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list bulk apply runs")
		return
	}
	runs := make([]bulkApplyRunSummary, 0, len(rows))
	for i := range rows {
		runs = append(runs, decodeBulkApplyRunSummary(&rows[i]))
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": vaultHash,
		"workflow_name":      workflowName,
		"count":              len(runs),
		"runs":               runs,
	})
}

func (s *Server) handleBulkApplyRun(w http.ResponseWriter, r *http.Request) {
	runID := pathVal(r, "run")
	if runID == "" {
		s.respondError(w, http.StatusBadRequest, "run is required")
		return
	}
	run, err := s.db.GetBulkApplyRun(runID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, decodeBulkApplyRunSummary(run))
}

func (s *Server) proxyBulkApplyWorkflow(r *http.Request, vaultHash, workflowName, path string) (int, []byte, error) {
	agent, err := s.findAgent(vaultHash)
	if err != nil {
		return http.StatusNotFound, nil, err
	}
	payload, err := s.buildBulkApplyWorkflowPayload(vaultHash, workflowName)
	if err != nil {
		return http.StatusBadRequest, nil, err
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, agent.URL()+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusBadGateway, nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, nil
}

func (s *Server) proxyAndRecordWorkflow(w http.ResponseWriter, r *http.Request, endpoint, runType, failStatus string) {
	vaultHash := pathVal(r, "vault")
	workflowName := pathVal(r, "name")
	if vaultHash == "" || workflowName == "" {
		s.respondError(w, http.StatusBadRequest, "vault and workflow are required")
		return
	}
	statusCode, body, err := s.proxyBulkApplyWorkflow(r, vaultHash, workflowName, endpoint)
	if err != nil {
		s.respondError(w, statusCode, err.Error())
		return
	}
	var payload map[string]any
	status := failStatus
	if json.Unmarshal(body, &payload) == nil {
		if raw := strings.TrimSpace(fmt.Sprint(payload["status"])); raw != "" {
			status = raw
		}
	}
	s.saveBulkApplyRun(vaultHash, workflowName, runType, status, body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

func (s *Server) handleBulkApplyWorkflowPrecheck(w http.ResponseWriter, r *http.Request) {
	s.proxyAndRecordWorkflow(w, r, "/api/bulk-apply/precheck", "precheck", "precheck_failed")
}

func (s *Server) handleBulkApplyWorkflowRun(w http.ResponseWriter, r *http.Request) {
	s.proxyAndRecordWorkflow(w, r, "/api/bulk-apply/execute", "run", "apply_failed")
}
