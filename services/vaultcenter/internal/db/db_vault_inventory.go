package db

import "time"

func (d *DB) UpsertVaultInventoryFromAgent(agent *Agent) error {
	if agent == nil {
		return nil
	}

	displayName := agent.VaultName
	status := "ok"
	runtimeHash := agent.AgentHash
	if runtimeHash == "" {
		runtimeHash = agent.NodeID
	}
	if agent.BlockedAt != nil {
		status = "blocked"
	} else if agent.RotationRequired {
		status = "rotation_required"
	} else if agent.RebindRequired {
		status = "rebind_required"
	}

	inventory := VaultInventory{
		VaultNodeUUID:    agent.NodeID,
		VaultRuntimeHash: runtimeHash,
		VaultHash:        agent.VaultHash,
		VaultName:        agent.VaultName,
		DisplayName:      displayName,
		ManagedPathsJSON: agent.ManagedPaths,
		Mode:             "localvault",
		Status:           status,
		Blocked:          agent.BlockedAt != nil,
		RotationRequired: agent.RotationRequired,
		RebindRequired:   agent.RebindRequired,
		FirstSeenAt:      agent.FirstSeen,
		LastSeenAt:       agent.LastSeen,
		UpdatedAt:        time.Now().UTC(),
	}
	return d.conn.Save(&inventory).Error
}

func (d *DB) BackfillVaultInventoryFromAgents() (int, error) {
	agents, err := d.ListAgents()
	if err != nil {
		return 0, err
	}

	count := 0
	for i := range agents {
		if err := d.UpsertVaultInventoryFromAgent(&agents[i]); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (d *DB) ListVaultInventory() ([]VaultInventory, error) {
	var rows []VaultInventory
	err := d.conn.Order("last_seen_at DESC").Find(&rows).Error
	return rows, err
}

func (d *DB) ListVaultInventoryFiltered(status, vaultHash string, limit, offset int) ([]VaultInventory, int64, error) {
	query := d.conn.Model(&VaultInventory{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
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

	var rows []VaultInventory
	err := query.Order("last_seen_at DESC").Find(&rows).Error
	return rows, total, err
}

func (d *DB) GetVaultInventoryByNodeID(nodeID string) (*VaultInventory, error) {
	var row VaultInventory
	if err := d.conn.First(&row, "vault_node_uuid = ?", nodeID).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (d *DB) UpdateVaultInventoryMeta(nodeID, displayName, description, tagsJSON string) error {
	return d.conn.Model(&VaultInventory{}).
		Where("vault_node_uuid = ?", nodeID).
		Select("DisplayName", "Description", "TagsJSON").
		Updates(&VaultInventory{DisplayName: displayName, Description: description, TagsJSON: tagsJSON}).Error
}
