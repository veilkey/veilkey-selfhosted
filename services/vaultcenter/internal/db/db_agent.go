package db

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/veilkey/veilkey-go-package/agentapi"
)

// AgentRetrySchedule defines backoff intervals for rebind/rotation retries.
var AgentRetrySchedule = []time.Duration{
	time.Minute,
	3 * time.Minute,
	10 * time.Minute,
}

func generateNextAgentHash() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (d *DB) UpsertAgent(nodeID, label, vaultHash, vaultName, ip string, port, secretsCount, configsCount, version, keyVersion int) error {
	if port == 0 {
		port = agentapi.DefaultPort
	}
	if keyVersion == 0 {
		keyVersion = 1
	}
	var existing Agent
	err := d.conn.First(&existing, "node_id = ?", nodeID).Error
	if err != nil {
		agent := Agent{
			NodeID:       nodeID,
			Label:        label,
			VaultHash:    vaultHash,
			VaultName:    vaultName,
			AgentRole:    "agent",
			HostEnabled:  false,
			LocalEnabled: true,
			KeyVersion:   keyVersion,
			IP:           ip,
			Port:         port,
			SecretsCount: secretsCount,
			ConfigsCount: configsCount,
			Version:      version,
		}
		if err := d.conn.Create(&agent).Error; err != nil {
			return err
		}
		return d.UpsertVaultInventoryFromAgent(&agent)
	}
	if strings.TrimSpace(existing.AgentRole) == "" {
		existing.AgentRole = DefaultAgentRole
	}
	existing.Label = label
	existing.VaultHash = vaultHash
	existing.VaultName = vaultName
	existing.KeyVersion = keyVersion
	existing.IP = ip
	existing.Port = port
	existing.SecretsCount = secretsCount
	existing.ConfigsCount = configsCount
	existing.Version = version
	if err := d.conn.Save(&existing).Error; err != nil {
		return err
	}
	updated, err := d.GetAgentByNodeID(nodeID)
	if err != nil {
		return err
	}
	if err := d.UpsertVaultInventoryFromAgent(updated); err != nil {
		return err
	}
	return d.BackfillSecretCatalogForAgent(updated.AgentHash)
}

func normalizeAgentCapabilities(role string, hostEnabled, localEnabled *bool) (bool, bool) {
	if hostEnabled != nil || localEnabled != nil {
		host := false
		local := true
		if hostEnabled != nil {
			host = *hostEnabled
		}
		if localEnabled != nil {
			local = *localEnabled
		}
		if hostEnabled == nil {
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "host-only", "host+local", "host-local", "dual":
				host = true
			}
		}
		if localEnabled == nil {
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "host-only":
				local = false
			default:
				local = true
			}
		}
		return host, local
	}
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "host-only":
		return true, false
	case "host+local", "host-local", "dual":
		return true, true
	default:
		return false, true
	}
}

func canonicalAgentRole(role string, host, local bool) string {
	if role = strings.TrimSpace(role); role != "" {
		return role
	}
	switch {
	case host && local:
		return "host+local"
	case host:
		return "host-only"
	default:
		return DefaultAgentRole
	}
}

func (d *DB) UpdateAgentRole(nodeID, role string) error {
	role = strings.TrimSpace(role)
	if role == "" {
		role = DefaultAgentRole
	}
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Update("agent_role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	updated, err := d.GetAgentByNodeID(nodeID)
	if err != nil {
		return err
	}
	if err := d.UpsertVaultInventoryFromAgent(updated); err != nil {
		return err
	}
	return d.BackfillSecretCatalogForAgent(updated.AgentHash)
}

func (d *DB) UpdateAgentCapabilities(nodeID, role string, hostEnabled, localEnabled *bool) error {
	host, local := normalizeAgentCapabilities(role, hostEnabled, localEnabled)
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Select("AgentRole", "HostEnabled", "LocalEnabled").
		Updates(&Agent{AgentRole: canonicalAgentRole(role, host, local), HostEnabled: host, LocalEnabled: local})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	updated, err := d.GetAgentByNodeID(nodeID)
	if err != nil {
		return err
	}
	if err := d.UpsertVaultInventoryFromAgent(updated); err != nil {
		return err
	}
	return d.BackfillSecretCatalogForAgent(updated.AgentHash)
}

func (d *DB) BackfillAgentCapabilities() error {
	agents, err := d.ListAgents()
	if err != nil {
		return err
	}
	for _, agent := range agents {
		role := strings.ToLower(strings.TrimSpace(agent.AgentRole))
		if role == "" || role == DefaultAgentRole {
			continue
		}
		host, local := normalizeAgentCapabilities(agent.AgentRole, nil, nil)
		if err := d.conn.Model(&Agent{}).Where("node_id = ?", agent.NodeID).
			Select("AgentRole", "HostEnabled", "LocalEnabled").
			Updates(&Agent{AgentRole: canonicalAgentRole(agent.AgentRole, host, local), HostEnabled: host, LocalEnabled: local}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) ListAgents() ([]Agent, error) {
	var agents []Agent
	err := d.conn.Where("archived_at IS NULL AND deleted_at IS NULL").Order("last_seen DESC").Find(&agents).Error
	return agents, err
}

func (d *DB) ListAgentsIncludeArchived() ([]Agent, error) {
	var agents []Agent
	err := d.conn.Where("deleted_at IS NULL").Order("last_seen DESC").Find(&agents).Error
	return agents, err
}

func (d *DB) ArchiveAgent(nodeID string) error {
	now := time.Now().UTC()
	return d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Update("archived_at", &now).Error
}

func (d *DB) UnarchiveAgent(nodeID string) error {
	return d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Update("archived_at", nil).Error
}

// AutoArchiveStaleAgents archives agents with no heartbeat for the given duration.
// Returns the number of agents archived.
func (d *DB) AutoArchiveStaleAgents(staleAfter time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-staleAfter)
	now := time.Now().UTC()
	result := d.conn.Model(&Agent{}).
		Where("archived_at IS NULL AND last_seen < ?", cutoff).
		Update("archived_at", &now)
	return result.RowsAffected, result.Error
}

func (d *DB) GetAgentByNodeID(nodeID string) (*Agent, error) {
	return dbFirst[Agent](d, "agent "+nodeID+" not found", "node_id = ?", nodeID)
}
func (d *DB) GetAgentByLabel(label string) (*Agent, error) {
	return dbFirst[Agent](d, "agent label "+label+" not found", "label = ?", label)
}
func (d *DB) GetAgentByHash(agentHash string) (*Agent, error) {
	return dbFirst[Agent](d, "agent hash "+agentHash+" not found", "agent_hash = ?", agentHash)
}
func (d *DB) GetAgentBySecretHash(secretHash string) (*Agent, error) {
	return dbFirst[Agent](d, "agent with secret hash not found", "agent_secret_hash = ?", secretHash)
}

func (d *DB) UpdateAgentSecretHash(nodeID, secretHash string, encSecret, encNonce []byte) error {
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Select("AgentSecretHash", "AgentSecretEnc", "AgentSecretNonce").
		Updates(&Agent{AgentSecretHash: secretHash, AgentSecretEnc: encSecret, AgentSecretNonce: encNonce})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	return nil
}

func (d *DB) DeleteAgentByNodeID(nodeID string) error {
	now := time.Now().UTC()
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Update("deleted_at", &now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	return nil
}

func (d *DB) RestoreDeletedAgent(nodeID string) error {
	return d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Update("deleted_at", nil).Error
}

func (d *DB) GetAgentRecord(hashOrLabel string) (*Agent, error) {
	if len(hashOrLabel) == 8 {
		if agent, err := d.GetAgentByHash(hashOrLabel); err == nil {
			return agent, nil
		}
	}
	return d.GetAgentByLabel(hashOrLabel)
}

func (d *DB) UpdateAgentDEK(nodeID string, agentHash string, dek, dekNonce []byte) error {
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Select("AgentHash", "DEK", "DEKNonce").
		Updates(&Agent{AgentHash: agentHash, DEK: dek, DEKNonce: dekNonce})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	updated, err := d.GetAgentByNodeID(nodeID)
	if err != nil {
		return err
	}
	if err := d.UpsertVaultInventoryFromAgent(updated); err != nil {
		return err
	}
	return d.BackfillSecretCatalogForAgent(updated.AgentHash)
}

func (d *DB) CountAgents() (int, error) {
	var count int64
	err := d.conn.Model(&Agent{}).Count(&count).Error
	return int(count), err
}

func (d *DB) UpdateAgentManagedPaths(nodeID string, managedPaths []string) error {
	normalized := normalizeManagedPaths(managedPaths)
	encoded, err := encodeManagedPaths(normalized)
	if err != nil {
		return err
	}
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Update("managed_paths", encoded)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	updated, err := d.GetAgentByNodeID(nodeID)
	if err != nil {
		return err
	}
	if err := d.UpsertVaultInventoryFromAgent(updated); err != nil {
		return err
	}
	return d.BackfillSecretCatalogForAgent(updated.AgentHash)
}

func (d *DB) ValidateManagedPaths(nodeID string, managedPaths []string) error {
	_ = nodeID
	_ = managedPaths
	return nil
}

func DecodeManagedPaths(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil
	}
	return normalizeManagedPaths(paths)
}

func encodeManagedPaths(paths []string) (string, error) {
	normalized := normalizeManagedPaths(paths)
	if len(normalized) == 0 {
		return "", nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeManagedPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			continue
		}
		path = filepath.Clean(path)
		if path == "." || path == "/" {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func (d *DB) AdvanceAgentRebind(nodeID, reason string, now time.Time) (*Agent, error) {
	var agent Agent
	if err := d.conn.First(&agent, "node_id = ?", nodeID).Error; err != nil {
		return nil, fmt.Errorf("agent %s not found", nodeID)
	}

	if agent.BlockedAt != nil {
		return &agent, nil
	}

	updates := map[string]interface{}{
		"rebind_required": true,
		"rebind_reason":   reason,
	}

	if agent.RetryStage >= len(AgentRetrySchedule) {
		updates["blocked_at"] = now
		updates["block_reason"] = reason
		updates["next_retry_at"] = nil
	} else {
		nextRetryAt := now.Add(AgentRetrySchedule[agent.RetryStage])
		updates["retry_stage"] = agent.RetryStage + 1
		updates["next_retry_at"] = &nextRetryAt
	}

	if err := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return d.GetAgentByNodeID(nodeID)
}

func (d *DB) ApproveAgentRebind(nodeID string) (*Agent, error) {
	var agent Agent
	if err := d.conn.First(&agent, "node_id = ?", nodeID).Error; err != nil {
		return nil, fmt.Errorf("agent %s not found", nodeID)
	}
	newAgentHash, err := generateNextAgentHash()
	if err != nil {
		return nil, err
	}
	agent.AgentHash = newAgentHash
	agent.KeyVersion++
	agent.RebindRequired = false
	agent.RebindReason = ""
	agent.RetryStage = 0
	agent.NextRetryAt = nil
	agent.BlockedAt = nil
	agent.BlockReason = ""
	if err := d.conn.Save(&agent).Error; err != nil {
		return nil, err
	}
	return d.GetAgentByNodeID(nodeID)
}

func (d *DB) ClearAgentRebind(nodeID string) (*Agent, error) {
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Select("RebindRequired", "RebindReason", "RetryStage", "NextRetryAt", "BlockedAt", "BlockReason").
		Updates(&Agent{})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("agent %s not found", nodeID)
	}
	return d.GetAgentByNodeID(nodeID)
}

func (d *DB) ScheduleAgentRotation(nodeID, reason string) (*Agent, error) {
	var agent Agent
	if err := d.conn.First(&agent, "node_id = ?", nodeID).Error; err != nil {
		return nil, fmt.Errorf("agent %s not found", nodeID)
	}
	agent.KeyVersion++
	agent.RotationRequired = true
	agent.RotationReason = reason
	agent.RebindRequired = false
	agent.RebindReason = ""
	agent.RetryStage = 0
	agent.NextRetryAt = nil
	agent.BlockedAt = nil
	agent.BlockReason = ""
	if err := d.conn.Save(&agent).Error; err != nil {
		return nil, err
	}
	return d.GetAgentByNodeID(nodeID)
}

func (d *DB) ClearAgentRotation(nodeID string) (*Agent, error) {
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Select("RotationRequired", "RotationReason").
		Updates(&Agent{})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("agent %s not found", nodeID)
	}
	return d.GetAgentByNodeID(nodeID)
}

// AgentStatePartialUpdate holds partial agent state fields for GORM updates.
// Pointer fields: nil = no change, non-nil = set to value.
type AgentStatePartialUpdate struct {
	RotationRequired *bool
	RotationReason   *string
	RebindRequired   *bool
	RebindReason     *string
	RetryStage       *int
	NextRetryAt      *time.Time
	SetNextRetryAt   bool // if true, apply NextRetryAt (nil = clear)
	BlockedAt        *time.Time
	SetBlockedAt     bool // if true, apply BlockedAt (nil = clear)
	BlockReason      *string
	KeyVersion       *int
}

// UpdateAgentStatePartial applies partial state updates using map-based GORM updates
// to correctly handle zeroing/nil-clearing of fields.
func (d *DB) UpdateAgentStatePartial(nodeID string, u *AgentStatePartialUpdate) error {
	m := map[string]interface{}{}
	if u.RotationRequired != nil {
		m["rotation_required"] = *u.RotationRequired
	}
	if u.RotationReason != nil {
		m["rotation_reason"] = *u.RotationReason
	}
	if u.RebindRequired != nil {
		m["rebind_required"] = *u.RebindRequired
	}
	if u.RebindReason != nil {
		m["rebind_reason"] = *u.RebindReason
	}
	if u.RetryStage != nil {
		m["retry_stage"] = *u.RetryStage
	}
	if u.SetNextRetryAt {
		m["next_retry_at"] = u.NextRetryAt // nil clears the column
	}
	if u.SetBlockedAt {
		m["blocked_at"] = u.BlockedAt // nil clears the column
	}
	if u.BlockReason != nil {
		m["block_reason"] = *u.BlockReason
	}
	if u.KeyVersion != nil {
		m["key_version"] = *u.KeyVersion
	}
	if len(m) == 0 {
		return nil
	}
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).Updates(m)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	return nil
}

func (d *DB) UpdateAgentRotationState(nodeID string, retryStage int, nextRetryAt *time.Time, rotationRequired bool, rotationReason string, blockedAt *time.Time, blockReason string) error {
	result := d.conn.Model(&Agent{}).Where("node_id = ?", nodeID).
		Select("RetryStage", "NextRetryAt", "RotationRequired", "RotationReason", "BlockedAt", "BlockReason").
		Updates(&Agent{
			RetryStage:       retryStage,
			NextRetryAt:      nextRetryAt,
			RotationRequired: rotationRequired,
			RotationReason:   rotationReason,
			BlockedAt:        blockedAt,
			BlockReason:      blockReason,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("agent %s not found", nodeID)
	}
	return nil
}

func (d *DB) ScheduleAllAgentRotations(reason string) ([]Agent, error) {
	agents, err := d.ListAgents()
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.BlockedAt != nil {
			continue
		}
		if agent.RotationRequired {
			continue
		}
		updated, err := d.ScheduleAgentRotation(agent.NodeID, reason)
		if err != nil {
			return nil, err
		}
		out = append(out, *updated)
	}
	return out, nil
}

func (d *DB) AdvancePendingRotations(now time.Time) ([]Agent, error) {
	agents, err := d.ListAgents()
	if err != nil {
		return nil, err
	}
	advanced := make([]Agent, 0)
	for _, agent := range agents {
		if !agent.RotationRequired || agent.BlockedAt != nil {
			continue
		}
		if agent.NextRetryAt != nil && agent.NextRetryAt.After(now) {
			continue
		}
		updates := map[string]interface{}{}
		if agent.RetryStage >= len(AgentRetrySchedule) {
			updates["blocked_at"] = now
			updates["block_reason"] = "rotation_timeout"
			updates["next_retry_at"] = nil
			updates["rotation_required"] = false
			updates["rotation_reason"] = ""
		} else {
			nextRetryAt := now.Add(AgentRetrySchedule[agent.RetryStage])
			updates["retry_stage"] = agent.RetryStage + 1
			updates["next_retry_at"] = &nextRetryAt
		}
		if err := d.conn.Model(&Agent{}).Where("node_id = ?", agent.NodeID).Updates(updates).Error; err != nil {
			return nil, err
		}
		updated, err := d.GetAgentByNodeID(agent.NodeID)
		if err != nil {
			return nil, err
		}
		advanced = append(advanced, *updated)
	}
	return advanced, nil
}
