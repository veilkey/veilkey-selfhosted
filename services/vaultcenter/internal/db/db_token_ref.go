package db

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type RefParts struct {
	Family string
	Scope  RefScope
	ID     string
}

func (r RefParts) Canonical() string {
	return r.Family + RefSep + string(r.Scope) + RefSep + r.ID
}

func ParseCanonicalRef(canonical string) (RefParts, error) {
	raw := strings.TrimSpace(canonical)
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return RefParts{}, fmt.Errorf("invalid canonical ref: %s", canonical)
	}
	parsed := RefParts{
		Family: parts[0],
		Scope:  RefScope(parts[1]),
		ID:     parts[2],
	}
	if err := parsed.Validate(); err != nil {
		return RefParts{}, err
	}
	return parsed, nil
}

func (r RefParts) Validate() error {
	r.Family = strings.ToUpper(strings.TrimSpace(r.Family))
	r.Scope = RefScope(strings.ToUpper(strings.TrimSpace(string(r.Scope))))
	r.ID = strings.TrimSpace(r.ID)
	if r.Family == "" {
		return fmt.Errorf("ref family is required")
	}
	if r.Scope == "" {
		return fmt.Errorf("ref scope is required")
	}
	if r.ID == "" {
		return fmt.Errorf("ref id is required")
	}
	return nil
}

func (d *DB) SaveRef(parts RefParts, ciphertext string, version int, status RefStatus, agentHash string) error {
	return d.SaveRefWithName(parts, ciphertext, version, status, agentHash, "")
}

func (d *DB) SaveRefWithName(parts RefParts, ciphertext string, version int, status RefStatus, agentHash string, secretName string) error {
	if err := parts.Validate(); err != nil {
		return err
	}
	if status == "" {
		if parts.Scope == RefScopeLocal || parts.Scope == RefScopeExternal {
			status = RefStatusActive
		} else {
			status = RefStatusTemp
		}
	}
	secretName = strings.TrimSpace(secretName)
	ref := TokenRef{
		RefCanonical: parts.Canonical(),
		RefFamily:    parts.Family,
		RefScope:     parts.Scope,
		RefID:        parts.ID,
		SecretName:   secretName,
		AgentHash:    agentHash,
		Ciphertext:   ciphertext,
		Version:      version,
		Status:       status,
	}
	if ref.SecretName == "" {
		ref.SecretName = ref.RefID
	}
	if err := d.conn.Exec(`
INSERT INTO token_refs (
	ref_canonical, ref_family, ref_scope, ref_id, secret_name, agent_hash, ciphertext, version, status, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(ref_canonical) DO UPDATE SET
	ref_family = excluded.ref_family,
	ref_scope = excluded.ref_scope,
	ref_id = excluded.ref_id,
	secret_name = CASE
		WHEN excluded.secret_name <> '' THEN excluded.secret_name
		ELSE token_refs.secret_name
	END,
	agent_hash = excluded.agent_hash,
	ciphertext = excluded.ciphertext,
	version = excluded.version,
	status = excluded.status
`, ref.RefCanonical, ref.RefFamily, ref.RefScope, ref.RefID, ref.SecretName, ref.AgentHash, ref.Ciphertext, ref.Version, ref.Status).Error; err != nil {
		return err
	}
	return d.UpsertSecretCatalogFromTrackedRef(&ref)
}

func (d *DB) NormalizeTokenRefStorage() error {
	var refs []TokenRef
	if err := d.conn.Order("ref_canonical ASC").Find(&refs).Error; err != nil {
		return err
	}

	for _, ref := range refs {
		parsed, err := ParseCanonicalRef(ref.RefCanonical)
		if err != nil {
			return err
		}

		needsBackfill := ref.RefFamily == "" || ref.RefScope == "" || ref.RefID == ""
		if needsBackfill {
			if err := d.conn.Model(&TokenRef{}).
				Where("ref_canonical = ?", ref.RefCanonical).
				Select("RefFamily", "RefScope", "RefID").
				Updates(&TokenRef{RefFamily: parsed.Family, RefScope: parsed.Scope, RefID: parsed.ID}).Error; err != nil {
				return err
			}
			continue
		}

		if ref.RefFamily != parsed.Family || ref.RefScope != parsed.Scope || ref.RefID != parsed.ID {
			return fmt.Errorf(
				"token ref storage mismatch for %s: columns=%s/%s/%s",
				ref.RefCanonical,
				ref.RefFamily,
				ref.RefScope,
				ref.RefID,
			)
		}
	}

	return nil
}

func DefaultRefStatus(scope RefScope) RefStatus {
	if scope == RefScopeLocal || scope == RefScopeExternal {
		return RefStatusActive
	}
	return RefStatusTemp
}

func (d *DB) GetRef(canonical string) (*TokenRef, error) {
	return dbFirst[TokenRef](d, "ref "+canonical+" not found", "ref_canonical = ?", canonical)
}

// GetRefBySecretName finds the first active (non-temp) ref with the given secret_name.
func (d *DB) GetRefBySecretName(name string) (*TokenRef, error) {
	return dbFirst[TokenRef](d, "ref for name "+name+" not found",
		"secret_name = ? AND ref_scope != ? AND status = ?", name, RefScopeTemp, RefStatusActive)
}

func (d *DB) ListRefs() ([]TokenRef, error) {
	var refs []TokenRef
	err := d.conn.Order("ref_canonical ASC").Find(&refs).Error
	return refs, err
}

func (d *DB) ListRefsByScope(scope RefScope) ([]TokenRef, error) {
	var refs []TokenRef
	err := d.conn.Where("ref_scope = ?", scope).Order("created_at DESC").Find(&refs).Error
	return refs, err
}

func (d *DB) ListRefsByVersion(version int) ([]TokenRef, error) {
	var refs []TokenRef
	err := d.conn.Where("version = ?", version).Find(&refs).Error
	return refs, err
}

func (d *DB) UpdateRef(canonical, newCiphertext string, newVersion int, newStatus RefStatus) error {
	return d.UpdateRefWithName(canonical, newCiphertext, newVersion, newStatus, "")
}

func (d *DB) UpdateRefWithName(canonical, newCiphertext string, newVersion int, newStatus RefStatus, secretName string) error {
	secretName = strings.TrimSpace(secretName)
	query := `
UPDATE token_refs
SET ciphertext = ?, version = ?`
	args := []any{newCiphertext, newVersion}
	if newStatus != "" {
		query += `, status = ?`
		args = append(args, newStatus)
	}
	if secretName != "" {
		query += `, secret_name = ?`
		args = append(args, secretName)
	}
	query += ` WHERE ref_canonical = ?`
	args = append(args, canonical)
	result := d.conn.Exec(query, args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("ref %s not found", canonical)
	}
	ref, err := d.GetRef(canonical)
	if err != nil {
		return err
	}
	return d.UpsertSecretCatalogFromTrackedRef(ref)
}

func (d *DB) UpdateRefAgentHash(canonical, agentHash string) error {
	return d.conn.Model(&TokenRef{}).Where("ref_canonical = ?", canonical).Update("agent_hash", agentHash).Error
}

func (d *DB) GetRefByCanonicalAndAgent(canonical, agentHash string) (*TokenRef, error) {
	var ref TokenRef
	err := d.conn.First(&ref, "ref_canonical = ? AND agent_hash = ?", canonical, agentHash).Error
	if err != nil {
		return nil, fmt.Errorf("ref %s not found for agent %s", canonical, agentHash)
	}
	return &ref, nil
}

func (d *DB) DeleteRef(canonical string) error {
	return dbDeleteWhere[TokenRef](d, "ref "+canonical+" not found", "ref_canonical = ?", canonical)
}
func (d *DB) CountRefs() (int, error) {
	var count int64
	err := d.conn.Model(&TokenRef{}).Count(&count).Error
	return int(count), err
}

func (d *DB) PromoteOperationalTempRefs(excludedAgentHashes map[string]bool) error {
	query := d.conn.Model(&TokenRef{}).
		Where("ref_scope = ? AND status = ?", RefScopeTemp, RefStatusTemp).
		Where("agent_hash <> ''")
	if len(excludedAgentHashes) > 0 {
		var hashes []string
		for hash := range excludedAgentHashes {
			hashes = append(hashes, hash)
		}
		query = query.Not("agent_hash IN ?", hashes)
	}
	if err := query.Where(`
		EXISTS (
			SELECT 1
			FROM token_refs existing
			WHERE existing.ref_family = token_refs.ref_family
			  AND existing.ref_scope = ?
			  AND existing.ref_id = token_refs.ref_id
		)`, "LOCAL").Delete(&TokenRef{}).Error; err != nil {
		return err
	}
	query = d.conn.Model(&TokenRef{}).
		Where("ref_scope = ? AND status = ?", RefScopeTemp, RefStatusTemp).
		Where("agent_hash <> ''")
	if len(excludedAgentHashes) > 0 {
		var hashes []string
		for hash := range excludedAgentHashes {
			hashes = append(hashes, hash)
		}
		query = query.Not("agent_hash IN ?", hashes)
	}
	return query.Updates(map[string]any{
		"ref_scope":     RefScopeLocal,
		"ref_canonical": gorm.Expr("ref_family || ':LOCAL:' || ref_id"),
		"status":        RefStatusActive,
	}).Error
}

func (d *DB) ListActiveTempRefs() ([]TokenRef, error) {
	var refs []TokenRef
	err := d.conn.Where("ref_scope = 'TEMP' AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)").
		Order("created_at DESC").Find(&refs).Error
	return refs, err
}

func (d *DB) FindActiveTempRefByHash(plaintextHash string) (*TokenRef, error) {
	var ref TokenRef
	err := d.conn.Where("plaintext_hash = ? AND ref_scope = 'TEMP' AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)", plaintextHash).First(&ref).Error
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (d *DB) SaveRefWithExpiry(parts RefParts, ciphertext string, version int, status RefStatus, expiresAt time.Time, secretName string) error {
	return d.SaveRefWithExpiryAndHash(parts, ciphertext, version, status, expiresAt, secretName, "")
}

func (d *DB) SaveRefWithExpiryAndHash(parts RefParts, ciphertext string, version int, status RefStatus, expiresAt time.Time, secretName string, plaintextHash string) error {
	if err := parts.Validate(); err != nil {
		return err
	}
	if status == "" {
		status = RefStatusTemp
	}
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		secretName = parts.ID
	}
	return d.conn.Exec(`
INSERT INTO token_refs (
	ref_canonical, ref_family, ref_scope, ref_id, secret_name, agent_hash, plaintext_hash, ciphertext, version, status, expires_at, created_at
) VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(ref_canonical) DO UPDATE SET
	ciphertext = excluded.ciphertext,
	version = excluded.version,
	status = excluded.status,
	expires_at = excluded.expires_at,
	secret_name = excluded.secret_name,
	plaintext_hash = excluded.plaintext_hash
`, parts.Canonical(), parts.Family, parts.Scope, parts.ID, secretName, plaintextHash, ciphertext, version, status, expiresAt).Error
}

func (d *DB) DeleteExpiredTempRefs() (int64, error) {
	result := d.conn.Exec(`DELETE FROM token_refs WHERE expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP`)
	return result.RowsAffected, result.Error
}

func (d *DB) EnsureTokenRefCanonicalUniqueness() error {
	if err := d.conn.Exec(`
DELETE FROM token_refs
WHERE rowid NOT IN (
	SELECT MAX(rowid)
	FROM token_refs
	GROUP BY ref_canonical
)`).Error; err != nil {
		return err
	}
	return d.conn.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_token_refs_ref_canonical
ON token_refs(ref_canonical)`).Error
}
