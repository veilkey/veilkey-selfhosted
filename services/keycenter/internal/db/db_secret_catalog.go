package db

import (
	"strings"
	"time"
)

func classifyCatalogEntry(ref *TokenRef) string {
	if ref == nil {
		return "key"
	}
	if ref.RefFamily == "VE" {
		return "config"
	}
	return "key"
}

func catalogNameFromRef(ref *TokenRef) string {
	if ref == nil {
		return ""
	}
	if name := strings.TrimSpace(ref.SecretName); name != "" {
		return name
	}
	return strings.TrimSpace(ref.RefID)
}

func (d *DB) UpsertSecretCatalogFromTrackedRef(ref *TokenRef) error {
	if ref == nil || ref.AgentHash == "" {
		return nil
	}

	agent, _ := d.GetAgentByHash(ref.AgentHash)
	if agent == nil {
		return nil // agent not registered yet — skip catalog sync
	}

	bindingCount, err := d.CountBindingsForRef(ref.RefCanonical)
	if err != nil {
		return err
	}

	name := catalogNameFromRef(ref)
	existing, _ := d.GetSecretCatalogByRef(ref.RefCanonical)
	displayName := name
	description := ""
	tagsJSON := "[]"
	class := classifyCatalogEntry(ref)
	fieldsPresentJSON := "[]"
	var lastRotatedAt, lastRevealedAt *time.Time
	if existing != nil {
		if strings.TrimSpace(existing.DisplayName) != "" {
			displayName = existing.DisplayName
		}
		description = existing.Description
		if strings.TrimSpace(existing.TagsJSON) != "" {
			tagsJSON = existing.TagsJSON
		}
		if strings.TrimSpace(existing.Class) != "" {
			class = existing.Class
		}
		if strings.TrimSpace(existing.FieldsPresentJSON) != "" {
			fieldsPresentJSON = existing.FieldsPresentJSON
		}
		lastRotatedAt = existing.LastRotatedAt
		lastRevealedAt = existing.LastRevealedAt
	}
	entry := &SecretCatalog{
		SecretCanonicalID: agent.VaultHash + ":" + ref.RefCanonical,
		SecretName:        name,
		DisplayName:       displayName,
		Description:       description,
		TagsJSON:          tagsJSON,
		Class:             class,
		Scope:             ref.RefScope,
		Status:            ref.Status,
		VaultNodeUUID:     agent.NodeID,
		VaultRuntimeHash:  agent.AgentHash,
		VaultHash:         agent.VaultHash,
		RefCanonical:      ref.RefCanonical,
		FieldsPresentJSON: fieldsPresentJSON,
		BindingCount:      bindingCount,
		LastRotatedAt:     lastRotatedAt,
		LastRevealedAt:    lastRevealedAt,
	}
	return d.SaveSecretCatalog(entry)
}

func (d *DB) BackfillSecretCatalogFromTrackedRefs() (int, error) {
	refs, err := d.ListRefs()
	if err != nil {
		return 0, err
	}

	count := 0
	for i := range refs {
		if refs[i].AgentHash == "" {
			continue
		}
		if err := d.UpsertSecretCatalogFromTrackedRef(&refs[i]); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (d *DB) BackfillSecretCatalogForAgent(agentHash string) error {
	if strings.TrimSpace(agentHash) == "" {
		return nil
	}

	refs, err := d.ListRefs()
	if err != nil {
		return err
	}
	for i := range refs {
		if refs[i].AgentHash != agentHash {
			continue
		}
		if err := d.UpsertSecretCatalogFromTrackedRef(&refs[i]); err != nil {
			return err
		}
	}
	return nil
}
