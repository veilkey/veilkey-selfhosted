package hkm

import (
	"time"

	"veilkey-vaultcenter/internal/db"

	chain "github.com/veilkey/veilkey-chain"
)

func ptr[T any](v T) *T { return &v }

// clearRebindPayload builds an UpdateAgentState payload that clears all rebind/block fields.
func clearRebindPayload(nodeID string) chain.UpdateAgentStatePayload {
	empty := ""
	return chain.UpdateAgentStatePayload{
		NodeID:         nodeID,
		RebindRequired: ptr(false),
		RebindReason:   &empty,
		RetryStage:     ptr(0),
		NextRetryAt:    &empty, // clear
		BlockedAt:      &empty, // clear
		BlockReason:    &empty,
	}
}

// clearRotationPayload builds an UpdateAgentState payload that clears rotation fields.
func clearRotationPayload(nodeID string) chain.UpdateAgentStatePayload {
	empty := ""
	return chain.UpdateAgentStatePayload{
		NodeID:           nodeID,
		RotationRequired: ptr(false),
		RotationReason:   &empty,
	}
}

// advanceRebindPayload computes the retry/block state and returns an UpdateAgentState payload.
// Mirrors the logic from db.AdvanceAgentRebind but keeps the decision in the handler.
func advanceRebindPayload(nodeID, reason string, currentRetryStage int, now time.Time) chain.UpdateAgentStatePayload {
	p := chain.UpdateAgentStatePayload{
		NodeID:         nodeID,
		RebindRequired: ptr(true),
		RebindReason:   &reason,
	}
	if currentRetryStage >= len(db.AgentRetrySchedule) {
		// Max retries exceeded → block
		blockedAt := now.UTC().Format(time.RFC3339)
		empty := ""
		p.BlockedAt = &blockedAt
		p.BlockReason = &reason
		p.NextRetryAt = &empty // clear
	} else {
		// Schedule next retry
		nextRetry := now.Add(db.AgentRetrySchedule[currentRetryStage]).UTC().Format(time.RFC3339)
		newStage := currentRetryStage + 1
		p.RetryStage = &newStage
		p.NextRetryAt = &nextRetry
	}
	return p
}

// advancePendingRotationPayload computes the rotation retry/block state for a single agent.
func advancePendingRotationPayload(nodeID string, currentRetryStage int, now time.Time) chain.UpdateAgentStatePayload {
	p := chain.UpdateAgentStatePayload{
		NodeID: nodeID,
	}
	if currentRetryStage >= len(db.AgentRetrySchedule) {
		// Max retries exceeded → block + clear rotation
		blockedAt := now.UTC().Format(time.RFC3339)
		empty := ""
		p.BlockedAt = &blockedAt
		p.BlockReason = ptr("rotation_timeout")
		p.NextRetryAt = &empty
		p.RotationRequired = ptr(false)
		p.RotationReason = &empty
	} else {
		nextRetry := now.Add(db.AgentRetrySchedule[currentRetryStage]).UTC().Format(time.RFC3339)
		newStage := currentRetryStage + 1
		p.RetryStage = &newStage
		p.NextRetryAt = &nextRetry
	}
	return p
}
