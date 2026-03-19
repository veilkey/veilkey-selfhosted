package db

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/veilkey/veilkey-go-package/crypto"

	"gorm.io/gorm"
)

func (d *DB) SaveSecretCatalog(entry *SecretCatalog) error {
	return d.conn.Save(entry).Error
}

func (d *DB) UpdateSecretCatalogMeta(refCanonical, displayName, description, tagsJSON string) error {
	return d.conn.Model(&SecretCatalog{}).
		Where("ref_canonical = ?", refCanonical).
		Select("DisplayName", "Description", "TagsJSON").
		Updates(&SecretCatalog{DisplayName: displayName, Description: description, TagsJSON: tagsJSON}).Error
}

func (d *DB) GetSecretCatalogByRef(refCanonical string) (*SecretCatalog, error) {
	var entry SecretCatalog
	if err := d.conn.First(&entry, "ref_canonical = ?", refCanonical).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

func (d *DB) CarrySecretCatalogIdentity(previousRefCanonical, newRefCanonical string) error {
	if strings.TrimSpace(previousRefCanonical) == "" || strings.TrimSpace(newRefCanonical) == "" || previousRefCanonical == newRefCanonical {
		return nil
	}

	previous, err := d.GetSecretCatalogByRef(previousRefCanonical)
	if err != nil {
		return err
	}

	current, err := d.GetSecretCatalogByRef(newRefCanonical)
	if err != nil {
		return err
	}

	current.SecretName = previous.SecretName
	if strings.TrimSpace(previous.DisplayName) != "" {
		current.DisplayName = previous.DisplayName
	}
	if strings.TrimSpace(previous.Description) != "" {
		current.Description = previous.Description
	}
	if strings.TrimSpace(previous.TagsJSON) != "" {
		current.TagsJSON = previous.TagsJSON
	}
	if strings.TrimSpace(previous.Class) != "" {
		current.Class = previous.Class
	}
	if strings.TrimSpace(previous.FieldsPresentJSON) != "" {
		current.FieldsPresentJSON = previous.FieldsPresentJSON
	}
	if previous.LastRotatedAt != nil {
		current.LastRotatedAt = previous.LastRotatedAt
	}
	if previous.LastRevealedAt != nil {
		current.LastRevealedAt = previous.LastRevealedAt
	}

	return d.SaveSecretCatalog(current)
}

func (d *DB) ListSecretCatalog() ([]SecretCatalog, error) {
	var rows []SecretCatalog
	err := d.conn.Order("vault_hash, secret_name").Find(&rows).Error
	return rows, err
}

func (d *DB) ListSecretCatalogFiltered(vaultHash, class, status, queryText string, limit, offset int) ([]SecretCatalog, int64, error) {
	query := d.conn.Model(&SecretCatalog{})
	if vaultHash != "" {
		query = query.Where("vault_hash = ?", vaultHash)
	}
	if class != "" {
		query = query.Where("class = ?", class)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if q := strings.TrimSpace(queryText); q != "" {
		like := "%" + q + "%"
		query = query.Where("secret_name LIKE ? OR display_name LIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var rows []SecretCatalog
	err := query.Order("vault_hash, secret_name").Find(&rows).Error
	return rows, total, err
}

func (d *DB) MarkSecretCatalogRevealed(refCanonical string, revealedAt time.Time) error {
	return d.conn.Model(&SecretCatalog{}).
		Where("ref_canonical = ?", refCanonical).
		Update("last_revealed_at", revealedAt.UTC()).Error
}

func (d *DB) RefreshSecretCatalogBindingCount(refCanonical string) error {
	count, err := d.CountBindingsForRef(refCanonical)
	if err != nil {
		return err
	}
	return d.conn.Model(&SecretCatalog{}).
		Where("ref_canonical = ?", refCanonical).
		Update("binding_count", count).Error
}

func (d *DB) RefreshAllSecretCatalogBindingCounts() error {
	rows, err := d.ListSecretCatalog()
	if err != nil {
		return err
	}
	for i := range rows {
		if err := d.RefreshSecretCatalogBindingCount(rows[i].RefCanonical); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) SaveBinding(entry *Binding) error {
	tx := d.conn.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	var before map[string]any
	var existing Binding
	created := false
	if err := tx.First(&existing, "binding_id = ?", entry.BindingID).Error; err == nil {
		before = bindingAuditPayload(existing)
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return err
	} else {
		created = true
	}

	if created {
		if err := tx.Model(&Binding{}).Create(map[string]any{
			"binding_id":    entry.BindingID,
			"binding_type":  entry.BindingType,
			"target_name":   entry.TargetName,
			"vault_hash":    entry.VaultHash,
			"secret_name":   entry.SecretName,
			"field_key":     entry.FieldKey,
			"ref_canonical": entry.RefCanonical,
			"required":      entry.Required,
		}).Error; err != nil {
			return err
		}
	} else {
		if err := tx.Select("*").Save(entry).Error; err != nil {
			return err
		}
	}
	if err := refreshSecretCatalogBindingCountTx(tx, entry.RefCanonical); err != nil {
		return err
	}
	if err := appendAuditEventTx(tx, "binding", entry.BindingID, "upsert", "system", entry.TargetName, "", "operator_catalog_db", before, bindingAuditPayload(*entry)); err != nil {
		return err
	}
	return tx.Commit().Error
}

func (d *DB) GetBinding(bindingID string) (*Binding, error) {
	var row Binding
	if err := d.conn.First(&row, "binding_id = ?", bindingID).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (d *DB) ListBindingsByTarget(bindingType, targetName string) ([]Binding, error) {
	rows, _, err := d.ListBindingsFiltered(bindingType, targetName, "", "", 0, 0)
	return rows, err
}

func (d *DB) ListBindingsFiltered(bindingType, targetName, vaultHash, refCanonical string, limit, offset int) ([]Binding, int64, error) {
	query := d.conn.Model(&Binding{}).Where("binding_type = ? AND target_name = ?", bindingType, targetName)
	if vaultHash != "" {
		query = query.Where("vault_hash = ?", vaultHash)
	}
	if refCanonical != "" {
		query = query.Where("ref_canonical = ?", refCanonical)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var rows []Binding
	err := query.Order("secret_name, field_key").Find(&rows).Error
	return rows, total, err
}

func (d *DB) ListBindingsByRefFiltered(refCanonical, vaultHash string, limit, offset int) ([]Binding, int64, error) {
	query := d.conn.Model(&Binding{}).Where("ref_canonical = ?", refCanonical)
	if vaultHash != "" {
		query = query.Where("vault_hash = ?", vaultHash)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var rows []Binding
	err := query.Order("binding_type, target_name, field_key").Find(&rows).Error
	return rows, total, err
}

func (d *DB) CountBindingsForRef(refCanonical string) (int, error) {
	var count int64
	err := d.conn.Model(&Binding{}).Where("ref_canonical = ?", refCanonical).Count(&count).Error
	return int(count), err
}

func (d *DB) DeleteBindingsByTarget(bindingType, targetName string) error {
	rows, err := d.ListBindingsByTarget(bindingType, targetName)
	if err != nil {
		return err
	}

	tx := d.conn.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	if err := tx.Where("binding_type = ? AND target_name = ?", bindingType, targetName).Delete(&Binding{}).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := refreshSecretCatalogBindingCountTx(tx, row.RefCanonical); err != nil {
			return err
		}
		if err := appendAuditEventTx(tx, "binding", row.BindingID, "delete", "system", targetName, "deleted_by_target", "operator_catalog_db", bindingAuditPayload(row), map[string]any{
			"binding_type": bindingType,
			"target_name":  targetName,
		}); err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

func (d *DB) DeleteBinding(bindingID string) error {
	row, err := d.GetBinding(bindingID)
	if err != nil {
		return err
	}

	tx := d.conn.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	if err := tx.Delete(&Binding{}, "binding_id = ?", bindingID).Error; err != nil {
		return err
	}
	if err := refreshSecretCatalogBindingCountTx(tx, row.RefCanonical); err != nil {
		return err
	}
	if err := appendAuditEventTx(tx, "binding", row.BindingID, "delete", "system", row.TargetName, "deleted_by_binding_id", "operator_catalog_db", bindingAuditPayload(*row), map[string]any{
		"binding_id":   row.BindingID,
		"binding_type": row.BindingType,
		"target_name":  row.TargetName,
	}); err != nil {
		return err
	}
	return tx.Commit().Error
}

func (d *DB) ReplaceBindingsForTarget(bindingType, targetName string, entries []Binding) error {
	tx := d.conn.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	var existing []Binding
	if err := tx.Where("binding_type = ? AND target_name = ?", bindingType, targetName).Find(&existing).Error; err != nil {
		return err
	}
	if err := tx.Where("binding_type = ? AND target_name = ?", bindingType, targetName).Delete(&Binding{}).Error; err != nil {
		return err
	}

	refsToRefresh := make(map[string]struct{})
	for _, row := range existing {
		refsToRefresh[row.RefCanonical] = struct{}{}
	}
	for i := range entries {
		if err := tx.Save(&entries[i]).Error; err != nil {
			return err
		}
		refsToRefresh[entries[i].RefCanonical] = struct{}{}
	}

	for refCanonical := range refsToRefresh {
		if err := refreshSecretCatalogBindingCountTx(tx, refCanonical); err != nil {
			return err
		}
	}
	for _, row := range existing {
		if err := appendAuditEventTx(tx, "binding", row.BindingID, "delete", "system", targetName, "replaced_by_target_set", "operator_catalog_db", bindingAuditPayload(row), map[string]any{
			"binding_type": bindingType,
			"target_name":  targetName,
		}); err != nil {
			return err
		}
	}
	for i := range entries {
		if err := appendAuditEventTx(tx, "binding", entries[i].BindingID, "upsert", "system", targetName, "replaced_target_set", "operator_catalog_db", nil, bindingAuditPayload(entries[i])); err != nil {
			return err
		}
	}
	return tx.Commit().Error
}

func (d *DB) SaveAuditEvent(entry *AuditEvent) error {
	return d.conn.Create(entry).Error
}

func (d *DB) ListAuditEvents(entityType, entityID string) ([]AuditEvent, error) {
	var rows []AuditEvent
	err := d.conn.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Order("created_at DESC").Find(&rows).Error
	return rows, err
}

func (d *DB) ListAuditEventsLimited(entityType, entityID string, limit, offset int) ([]AuditEvent, int64, error) {
	query := d.conn.Model(&AuditEvent{}).Where("entity_type = ? AND entity_id = ?", entityType, entityID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var rows []AuditEvent
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, total, err
}

func (d *DB) ListAuditEventsForVault(nodeID, agentHash string, limit, offset int) ([]AuditEvent, int64, error) {
	where := "entity_id = ? OR entity_id = ? OR actor_id = ? OR actor_id = ?"
	args := []any{nodeID, agentHash, nodeID, agentHash}

	var total int64
	if err := d.conn.Model(&AuditEvent{}).Where(where, args...).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := d.conn.Model(&AuditEvent{}).Where(where, args...)
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}

	var rows []AuditEvent
	err := q.Order("created_at DESC").Find(&rows).Error
	return rows, total, err
}

func (d *DB) ListRecentAdminAuditEvents(limit, offset int) ([]AuditEvent, int64, error) {
	query := d.conn.Model(&AuditEvent{}).Where(
		"entity_type IN ? OR action LIKE ? OR source LIKE ?",
		[]string{"admin_auth", "admin_session"},
		"admin_%",
		"admin_%",
	)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var rows []AuditEvent
	err := query.Order("created_at DESC").Find(&rows).Error
	return rows, total, err
}

func refreshSecretCatalogBindingCountTx(tx *gorm.DB, refCanonical string) error {
	if refCanonical == "" {
		return nil
	}
	var count int64
	if err := tx.Model(&Binding{}).Where("ref_canonical = ?", refCanonical).Count(&count).Error; err != nil {
		return err
	}
	return tx.Model(&SecretCatalog{}).
		Where("ref_canonical = ?", refCanonical).
		Update("binding_count", int(count)).Error
}

func appendAuditEventTx(tx *gorm.DB, entityType, entityID, action, actorType, actorID, reason, source string, before, after map[string]any) error {
	beforeJSON := "{}"
	if len(before) > 0 {
		data, err := json.Marshal(before)
		if err != nil {
			return err
		}
		beforeJSON = string(data)
	}

	afterJSON := "{}"
	if len(after) > 0 {
		data, err := json.Marshal(after)
		if err != nil {
			return err
		}
		afterJSON = string(data)
	}

	return tx.Save(&AuditEvent{
		EventID:    crypto.GenerateUUID(),
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		ActorType:  actorType,
		ActorID:    actorID,
		Reason:     reason,
		Source:     source,
		BeforeJSON: beforeJSON,
		AfterJSON:  afterJSON,
		CreatedAt:  time.Now().UTC(),
	}).Error
}

func bindingAuditPayload(row Binding) map[string]any {
	return map[string]any{
		"binding_id":    row.BindingID,
		"binding_type":  row.BindingType,
		"target_name":   row.TargetName,
		"vault_hash":    row.VaultHash,
		"secret_name":   row.SecretName,
		"field_key":     row.FieldKey,
		"ref_canonical": row.RefCanonical,
		"required":      row.Required,
	}
}
