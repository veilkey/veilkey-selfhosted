package db

import (
	"fmt"
	"strings"
)

var allowedBulkApplyFormats = map[string]struct{}{
	"env":        {},
	"json":       {},
	"json_merge": {},
	"line_patch": {},
	"raw":        {},
}

func normalizeBulkApplyTemplate(tmpl *BulkApplyTemplate) error {
	if tmpl == nil {
		return fmt.Errorf("bulk apply template is required")
	}
	tmpl.VaultRuntimeHash = strings.TrimSpace(tmpl.VaultRuntimeHash)
	tmpl.Name = strings.TrimSpace(tmpl.Name)
	tmpl.Format = strings.ToLower(strings.TrimSpace(tmpl.Format))
	tmpl.TargetPath = strings.TrimSpace(tmpl.TargetPath)
	tmpl.Hook = strings.TrimSpace(tmpl.Hook)
	if tmpl.VaultRuntimeHash == "" {
		return fmt.Errorf("vault_runtime_hash is required")
	}
	if tmpl.Name == "" {
		return fmt.Errorf("template name is required")
	}
	if tmpl.Format == "" {
		tmpl.Format = "env"
	}
	if _, ok := allowedBulkApplyFormats[tmpl.Format]; !ok {
		return fmt.Errorf("format must be one of: env, json, json_merge, line_patch, raw")
	}
	if tmpl.TargetPath == "" {
		return fmt.Errorf("target_path is required")
	}
	if tmpl.Body == "" {
		return fmt.Errorf("body is required")
	}
	if tmpl.TemplateID == "" {
		tmpl.TemplateID = MakeTemplateID(tmpl.VaultRuntimeHash, tmpl.Name)
	}
	return nil
}

func (d *DB) SaveBulkApplyTemplate(tmpl *BulkApplyTemplate) error {
	if err := normalizeBulkApplyTemplate(tmpl); err != nil {
		return err
	}
	return d.conn.Save(tmpl).Error
}

func (d *DB) ListBulkApplyTemplates(vaultRuntimeHash string) ([]BulkApplyTemplate, error) {
	var out []BulkApplyTemplate
	if err := d.conn.Where("vault_runtime_hash = ?", strings.TrimSpace(vaultRuntimeHash)).Order("name ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) GetBulkApplyTemplate(vaultRuntimeHash, name string) (*BulkApplyTemplate, error) {
	var tmpl BulkApplyTemplate
	if err := d.conn.First(&tmpl, "vault_runtime_hash = ? AND name = ?", strings.TrimSpace(vaultRuntimeHash), strings.TrimSpace(name)).Error; err != nil {
		return nil, fmt.Errorf("bulk apply template %s not found", strings.TrimSpace(name))
	}
	return &tmpl, nil
}

func (d *DB) DeleteBulkApplyTemplate(vaultRuntimeHash, name string) error {
	result := d.conn.Delete(&BulkApplyTemplate{}, "vault_runtime_hash = ? AND name = ?", strings.TrimSpace(vaultRuntimeHash), strings.TrimSpace(name))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("bulk apply template %s not found", strings.TrimSpace(name))
	}
	return nil
}
