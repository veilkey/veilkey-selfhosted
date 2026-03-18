package bulk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Shared file-level types
// ---------------------------------------------------------------------------

type bulkApplyTemplateFile struct {
	APIVersion       string `json:"apiVersion"`
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	VaultRuntimeHash string `json:"vaultRuntimeHash"`
	Format           string `json:"format"`
	TargetPath       string `json:"targetPath"`
	Hook             string `json:"hook,omitempty"`
	Enabled          bool   `json:"enabled"`
	BodyFile         string `json:"bodyFile"`
}

type bulkApplyWorkflowStepFile struct {
	Template string `json:"template"`
}

type bulkApplyWorkflowFile struct {
	APIVersion       string                      `json:"apiVersion"`
	Kind             string                      `json:"kind"`
	Name             string                      `json:"name"`
	VaultRuntimeHash string                      `json:"vaultRuntimeHash"`
	Label            string                      `json:"label"`
	Description      string                      `json:"description,omitempty"`
	Steps            []bulkApplyWorkflowStepFile `json:"steps"`
	Hooks            []string                    `json:"hooks,omitempty"`
}

type bulkApplyTemplateRecord struct {
	TemplateID        string `json:"template_id"`
	VaultRuntimeHash  string `json:"vault_runtime_hash"`
	Name              string `json:"name"`
	Format            string `json:"format"`
	TargetPath        string `json:"target_path"`
	Body              string `json:"body"`
	Hook              string `json:"hook"`
	Enabled           bool   `json:"enabled"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	ValidationStatus  string `json:"validation_status"`
	ValidationMessage string `json:"validation_message,omitempty"`
}

// bulkApplyTemplatePayload is the request body for create/update template.
type bulkApplyTemplatePayload struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	TargetPath string `json:"target_path"`
	Body       string `json:"body"`
	Hook       string `json:"hook"`
	Enabled    *bool  `json:"enabled"`
}

const (
	bulkApplyTemplateKind = "BulkApplyTemplate"
	bulkApplyWorkflowKind = "BulkApplyWorkflow"
)

var allowedBulkApplyFormatsFile = map[string]struct{}{
	"env":        {},
	"json":       {},
	"json_merge": {},
	"line_patch": {},
	"raw":        {},
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func (h *Handler) bulkApplyTemplatesDir(vaultHash string) string {
	return filepath.Join(h.deps.BulkApplyDir(), "templates", strings.TrimSpace(vaultHash))
}

func (h *Handler) bulkApplyWorkflowsDir(vaultHash string) string {
	return filepath.Join(h.deps.BulkApplyDir(), "workflows", strings.TrimSpace(vaultHash))
}

func (h *Handler) bulkApplySchemaDir() string {
	return filepath.Join(h.deps.BulkApplyDir(), "schema")
}

func (h *Handler) bulkApplyTemplateMetaPath(vaultHash, name string) string {
	return filepath.Join(h.bulkApplyTemplatesDir(vaultHash), strings.TrimSpace(name)+".json")
}

func (h *Handler) bulkApplyWorkflowPath(vaultHash, name string) string {
	return filepath.Join(h.bulkApplyWorkflowsDir(vaultHash), strings.TrimSpace(name)+".json")
}

// ---------------------------------------------------------------------------
// Low-level file utilities
// ---------------------------------------------------------------------------

func ensureFileWithContent(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return atomicWriteFile(path, content, 0o644)
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func decodeStrictJSON[T any](raw []byte, target *T) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func (h *Handler) ensureBulkApplyBase() error {
	for _, dir := range []string{
		h.deps.BulkApplyDir(),
		filepath.Join(h.deps.BulkApplyDir(), "templates"),
		filepath.Join(h.deps.BulkApplyDir(), "workflows"),
		h.bulkApplySchemaDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := ensureFileWithContent(filepath.Join(h.bulkApplySchemaDir(), "template.schema.json"), []byte(templateSchemaJSON)); err != nil {
		return err
	}
	if err := ensureFileWithContent(filepath.Join(h.bulkApplySchemaDir(), "workflow.schema.json"), []byte(workflowSchemaJSON)); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Template file persistence
// ---------------------------------------------------------------------------

func normalizeBulkApplyTemplatePayload(vaultHash string, req *bulkApplyTemplatePayload, existingName string) (*bulkApplyTemplateRecord, *bulkApplyTemplateFile, error) {
	if req == nil {
		return nil, nil, fmt.Errorf("template payload is required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(existingName)
	}
	if name == "" {
		return nil, nil, fmt.Errorf("template name is required")
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "env"
	}
	if _, ok := allowedBulkApplyFormatsFile[format]; !ok {
		return nil, nil, fmt.Errorf("format must be one of: env, json, json_merge, line_patch, raw")
	}
	targetPath := strings.TrimSpace(req.TargetPath)
	if targetPath == "" {
		return nil, nil, fmt.Errorf("target_path is required")
	}
	if strings.TrimSpace(req.Body) == "" {
		return nil, nil, fmt.Errorf("body is required")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	record := &bulkApplyTemplateRecord{
		TemplateID:       strings.TrimSpace(vaultHash) + ":" + name,
		VaultRuntimeHash: strings.TrimSpace(vaultHash),
		Name:             name,
		Format:           format,
		TargetPath:       targetPath,
		Body:             req.Body,
		Hook:             strings.TrimSpace(req.Hook),
		Enabled:          enabled,
	}
	meta := &bulkApplyTemplateFile{
		APIVersion:       "veilkey.io/v1",
		Kind:             bulkApplyTemplateKind,
		Name:             name,
		VaultRuntimeHash: strings.TrimSpace(vaultHash),
		Format:           format,
		TargetPath:       targetPath,
		Hook:             strings.TrimSpace(req.Hook),
		Enabled:          enabled,
		BodyFile:         name + ".body",
	}
	if format == "json" || format == "json_merge" {
		var js any
		if err := json.Unmarshal([]byte(req.Body), &js); err != nil {
			return nil, nil, fmt.Errorf("body must be valid JSON for format %s", format)
		}
	}
	return record, meta, nil
}

func (h *Handler) saveBulkApplyTemplateFile(vaultHash, existingName string, req *bulkApplyTemplatePayload) (*bulkApplyTemplateRecord, error) {
	record, meta, err := normalizeBulkApplyTemplatePayload(vaultHash, req, existingName)
	if err != nil {
		return nil, err
	}
	if err := h.ensureBulkApplyBase(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(existingName) != "" && strings.TrimSpace(existingName) != record.Name {
		_ = os.Remove(h.bulkApplyTemplateMetaPath(vaultHash, existingName))
		_ = os.Remove(filepath.Join(h.bulkApplyTemplatesDir(vaultHash), strings.TrimSpace(existingName)+".body"))
	}
	bodyPath := filepath.Join(h.bulkApplyTemplatesDir(vaultHash), meta.BodyFile)
	if err := atomicWriteFile(bodyPath, []byte(record.Body), 0o644); err != nil {
		return nil, err
	}
	metaRaw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	metaRaw = append(metaRaw, '\n')
	if err := atomicWriteFile(h.bulkApplyTemplateMetaPath(vaultHash, record.Name), metaRaw, 0o644); err != nil {
		return nil, err
	}
	record.ValidationStatus = "valid"
	return record, nil
}

func (h *Handler) loadBulkApplyTemplateRecord(vaultHash, name string) (*bulkApplyTemplateRecord, error) {
	metaPath := h.bulkApplyTemplateMetaPath(vaultHash, name)
	metaRaw, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("bulk apply template %s not found", strings.TrimSpace(name))
	}
	record := &bulkApplyTemplateRecord{
		TemplateID:       strings.TrimSpace(vaultHash) + ":" + strings.TrimSpace(name),
		VaultRuntimeHash: strings.TrimSpace(vaultHash),
		Name:             strings.TrimSpace(name),
		ValidationStatus: "valid",
	}
	var fileMeta bulkApplyTemplateFile
	if err := decodeStrictJSON(metaRaw, &fileMeta); err != nil {
		record.ValidationStatus = "parse_error"
		record.ValidationMessage = err.Error()
		return record, nil
	}
	if strings.TrimSpace(fileMeta.Kind) != bulkApplyTemplateKind {
		record.ValidationStatus = "schema_error"
		record.ValidationMessage = "kind must be BulkApplyTemplate"
		return record, nil
	}
	record.Name = strings.TrimSpace(fileMeta.Name)
	record.Format = strings.ToLower(strings.TrimSpace(fileMeta.Format))
	record.TargetPath = strings.TrimSpace(fileMeta.TargetPath)
	record.Hook = strings.TrimSpace(fileMeta.Hook)
	record.Enabled = fileMeta.Enabled
	if _, ok := allowedBulkApplyFormatsFile[record.Format]; !ok {
		record.ValidationStatus = "schema_error"
		record.ValidationMessage = "unsupported format"
		return record, nil
	}
	if strings.TrimSpace(fileMeta.BodyFile) == "" {
		record.ValidationStatus = "missing_body"
		record.ValidationMessage = "bodyFile is required"
		return record, nil
	}
	bodyRaw, err := os.ReadFile(filepath.Join(h.bulkApplyTemplatesDir(vaultHash), fileMeta.BodyFile))
	if err != nil {
		record.ValidationStatus = "missing_body"
		record.ValidationMessage = "body file not found"
		return record, nil
	}
	record.Body = string(bodyRaw)
	if record.Format == "json" || record.Format == "json_merge" {
		var js any
		if err := json.Unmarshal(bodyRaw, &js); err != nil {
			record.ValidationStatus = "schema_error"
			record.ValidationMessage = "body is not valid JSON"
			return record, nil
		}
	}
	return record, nil
}

func (h *Handler) listBulkApplyTemplateRecords(vaultHash string) ([]bulkApplyTemplateRecord, error) {
	if err := h.migrateBulkApplyTemplatesFromDB(vaultHash); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(h.bulkApplyTemplatesDir(vaultHash), 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(h.bulkApplyTemplatesDir(vaultHash))
	if err != nil {
		return nil, err
	}
	out := []bulkApplyTemplateRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		row, err := h.loadBulkApplyTemplateRecord(vaultHash, name)
		if err != nil {
			continue
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h *Handler) deleteBulkApplyTemplateFile(vaultHash, name string) error {
	row, err := h.loadBulkApplyTemplateRecord(vaultHash, name)
	if err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(h.bulkApplyTemplatesDir(vaultHash), row.Name+".body"))
	if err := os.Remove(h.bulkApplyTemplateMetaPath(vaultHash, row.Name)); err != nil {
		return err
	}
	return nil
}

func (h *Handler) loadBulkApplyWorkflowFile(vaultHash, name string) (*bulkApplyWorkflowFile, string, string, error) {
	path := h.bulkApplyWorkflowPath(vaultHash, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "broken", "workflow file not found", err
	}
	var workflow bulkApplyWorkflowFile
	if err := decodeStrictJSON(raw, &workflow); err != nil {
		return nil, "parse_error", err.Error(), err
	}
	if strings.TrimSpace(workflow.Kind) != bulkApplyWorkflowKind {
		return nil, "schema_error", "kind must be BulkApplyWorkflow", nil
	}
	if len(workflow.Steps) == 0 {
		return nil, "schema_error", "workflow must include at least one step", nil
	}
	for _, step := range workflow.Steps {
		if strings.TrimSpace(step.Template) == "" {
			return nil, "schema_error", "workflow step template is required", nil
		}
		row, err := h.loadBulkApplyTemplateRecord(vaultHash, step.Template)
		if err != nil {
			return nil, "missing_template", "referenced template not found", err
		}
		if row.ValidationStatus != "valid" {
			return nil, "missing_template", "referenced template is broken", nil
		}
	}
	return &workflow, "valid", "", nil
}

func (h *Handler) saveBulkApplyWorkflowFile(vaultHash string, workflow *bulkApplyWorkflowFile) error {
	if workflow == nil {
		return fmt.Errorf("workflow is required")
	}
	if err := h.ensureBulkApplyBase(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return atomicWriteFile(h.bulkApplyWorkflowPath(vaultHash, workflow.Name), raw, 0o644)
}

func (h *Handler) listBulkApplyWorkflowSummaries(vaultHash string) ([]bulkApplyWorkflowSummary, error) {
	if err := h.migrateBulkApplyTemplatesFromDB(vaultHash); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(h.bulkApplyWorkflowsDir(vaultHash), 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(h.bulkApplyWorkflowsDir(vaultHash))
	if err != nil {
		return nil, err
	}
	out := []bulkApplyWorkflowSummary{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		summary, err := h.buildBulkApplyWorkflowSummary(vaultHash, name)
		if err != nil {
			continue
		}
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h *Handler) migrateBulkApplyTemplatesFromDB(vaultHash string) error {
	if err := h.ensureBulkApplyBase(); err != nil {
		return err
	}
	hasJSON := false
	entries, err := os.ReadDir(h.bulkApplyTemplatesDir(vaultHash))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				hasJSON = true
				break
			}
		}
	}
	if !hasJSON {
		rows, err := h.deps.DB().ListBulkApplyTemplates(vaultHash)
		if err == nil && len(rows) > 0 {
			for _, row := range rows {
				enabled := row.Enabled
				payload := &bulkApplyTemplatePayload{
					Name:       row.Name,
					Format:     row.Format,
					TargetPath: row.TargetPath,
					Body:       row.Body,
					Hook:       row.Hook,
					Enabled:    &enabled,
				}
				if _, err := h.saveBulkApplyTemplateFile(vaultHash, row.Name, payload); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Embedded JSON schemas
// ---------------------------------------------------------------------------

const templateSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "BulkApplyTemplate",
  "type": "object",
  "additionalProperties": false,
  "required": ["apiVersion", "kind", "name", "vaultRuntimeHash", "format", "targetPath", "enabled", "bodyFile"],
  "properties": {
    "apiVersion": { "const": "veilkey.io/v1" },
    "kind": { "const": "BulkApplyTemplate" },
    "name": { "type": "string", "minLength": 1 },
    "vaultRuntimeHash": { "type": "string", "minLength": 1 },
    "format": { "enum": ["env", "json", "json_merge", "line_patch", "raw"] },
    "targetPath": { "type": "string", "minLength": 1 },
    "hook": { "type": "string" },
    "enabled": { "type": "boolean" },
    "bodyFile": { "type": "string", "minLength": 1 }
  }
}
`

const workflowSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "BulkApplyWorkflow",
  "type": "object",
  "additionalProperties": false,
  "required": ["apiVersion", "kind", "name", "vaultRuntimeHash", "label", "steps"],
  "properties": {
    "apiVersion": { "const": "veilkey.io/v1" },
    "kind": { "const": "BulkApplyWorkflow" },
    "name": { "type": "string", "minLength": 1 },
    "vaultRuntimeHash": { "type": "string", "minLength": 1 },
    "label": { "type": "string", "minLength": 1 },
    "description": { "type": "string" },
    "hooks": { "type": "array", "items": { "type": "string" } },
    "steps": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["template"],
        "properties": {
          "template": { "type": "string", "minLength": 1 }
        }
      }
    }
  }
}
`
