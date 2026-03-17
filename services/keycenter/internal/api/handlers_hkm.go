package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"veilkey-keycenter/internal/crypto"
)

// SetupHKMRoutes adds HKM-specific API routes
func (s *Server) SetupHKMRoutes(mux *http.ServeMux) {
	ready := s.requireReadyForOps
	// Parent API (called by root/parent to manage this node's children)
	mux.HandleFunc("POST /api/register", s.requireTrustedIP(ready(s.handleRegister)))
	mux.HandleFunc("POST /api/rekey", s.requireTrustedIP(ready(s.handleRekey)))
	mux.HandleFunc("GET /api/node-info", ready(s.handleNodeInfo))
	mux.HandleFunc("GET /api/children", ready(s.handleListChildren))
	mux.HandleFunc("DELETE /api/children/{node_id}", s.requireTrustedIP(ready(s.handleDeleteChild)))
	mux.HandleFunc("GET /api/registry", ready(s.handleListRegistry))

	// Resolve scoped VK ref → plaintext value
	mux.HandleFunc("GET /api/resolve/{ref}", ready(s.handleResolveSecret))

	// Child heartbeat (report URL)
	mux.HandleFunc("POST /api/heartbeat", s.requireTrustedIP(ready(s.handleHeartbeat)))

	// Set parent URL (called on child to configure parent resolve)
	mux.HandleFunc("POST /api/set-parent", s.requireTrustedIP(ready(s.handleSetParent)))

	// Key rotation (root triggers full tree rotation)
	mux.HandleFunc("POST /api/federation/rotate", s.requireTrustedIP(ready(s.handleFederatedRotate)))

	// Agent management (Hub-only decryption)
	mux.HandleFunc("POST /api/agents/heartbeat", ready(s.handleAgentHeartbeat))
	mux.HandleFunc("DELETE /api/agents/by-node/{node_id}", s.requireTrustedIP(ready(s.handleAgentUnregisterByNode)))
	mux.HandleFunc("GET /api/resolve-agent/{token}", ready(s.handleAgentResolve))
	mux.HandleFunc("GET /api/agents", ready(s.handleAgentList))
	mux.HandleFunc("GET /api/agents/{agent}/rebind-plan", s.requireTrustedIP(ready(s.handleAgentRebindPlan)))
	mux.HandleFunc("POST /api/agents/rotate-all", s.requireTrustedIP(ready(s.handleAgentRotateAll)))
	mux.HandleFunc("GET /api/tracked-refs/audit", ready(s.handleTrackedRefAudit))
	mux.HandleFunc("POST /api/tracked-refs/cleanup", s.requireTrustedIP(ready(s.handleTrackedRefCleanup)))
	mux.HandleFunc("POST /api/tracked-refs/sync", s.requireTrustedIP(ready(s.handleTrackedRefSync)))
	mux.HandleFunc("GET /api/vault-inventory", ready(s.handleVaultInventory))
	mux.HandleFunc("GET /api/catalog/secrets", ready(s.handleSecretCatalogList))
	mux.HandleFunc("GET /api/catalog/secrets/{ref}", ready(s.handleSecretCatalogGet))
	mux.HandleFunc("GET /api/catalog/bindings", ready(s.handleBindingsList))
	mux.HandleFunc("GET /api/catalog/audit", ready(s.handleAuditEventsList))
	mux.HandleFunc("GET /api/ref-policy", ready(s.handleRefPolicy))
	mux.HandleFunc("GET /api/targets/{binding_type}/{target_name}/bindings", ready(s.handleTargetBindings))
	mux.HandleFunc("PUT /api/targets/{binding_type}/{target_name}/bindings", s.requireTrustedIP(ready(s.handleTargetBindingsReplace)))
	mux.HandleFunc("DELETE /api/targets/{binding_type}/{target_name}/bindings", s.requireTrustedIP(ready(s.handleTargetBindingsDeleteAll)))
	mux.HandleFunc("GET /api/targets/{binding_type}/{target_name}/impact", ready(s.handleTargetImpact))
	mux.HandleFunc("GET /api/targets/{binding_type}/{target_name}/summary", ready(s.handleTargetSummary))
	mux.HandleFunc("GET /api/functions/global", ready(s.handleGlobalFunctions))
	mux.HandleFunc("POST /api/functions/global", s.requireTrustedIP(ready(s.handleGlobalFunctions)))
	mux.HandleFunc("GET /api/functions/global/{name}", ready(s.handleGlobalFunction))
	mux.HandleFunc("DELETE /api/functions/global/{name}", s.requireTrustedIP(ready(s.handleGlobalFunction)))
	mux.HandleFunc("POST /api/functions/global/{name}/run", s.requireTrustedIP(ready(s.handleGlobalFunctionRun)))
	mux.HandleFunc("GET /api/agents/{agent}/secrets", ready(s.handleAgentSecrets))
	mux.HandleFunc("GET /api/agents/{agent}/secrets/{name}", ready(s.handleAgentGetSecret))

	mux.HandleFunc("POST /api/agents/{agent}/secrets", s.requireTrustedIP(ready(s.handleAgentSaveSecret)))
	mux.HandleFunc("POST /api/agents/{agent}/secrets/{name}/fields", s.requireTrustedIP(ready(s.handleAgentSaveSecretFields)))
	mux.HandleFunc("GET /api/agents/{agent}/secrets/{name}/fields/{field}", ready(s.handleAgentGetSecretField))
	mux.HandleFunc("DELETE /api/agents/{agent}/secrets/{name}/fields/{field}", s.requireTrustedIP(ready(s.handleAgentDeleteSecretField)))
	mux.HandleFunc("DELETE /api/agents/{agent}/secrets/{name}", s.requireTrustedIP(ready(s.handleAgentDeleteSecret)))
	mux.HandleFunc("GET /api/vaults", ready(s.handleVaultList))
	mux.HandleFunc("GET /api/vaults/{vault}", ready(s.handleVaultGet))
	mux.HandleFunc("PATCH /api/vaults/{vault}", s.requireTrustedIP(ready(s.handleVaultPatch)))
	mux.HandleFunc("GET /api/vaults/{vault}/audit", ready(s.handleVaultAudit))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/templates", ready(s.handleBulkApplyTemplates))
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/templates", s.requireTrustedIP(ready(s.handleBulkApplyTemplates)))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/templates/{name}", ready(s.handleBulkApplyTemplate))
	mux.HandleFunc("PUT /api/vaults/{vault}/bulk-apply/templates/{name}", s.requireTrustedIP(ready(s.handleBulkApplyTemplate)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/bulk-apply/templates/{name}", s.requireTrustedIP(ready(s.handleBulkApplyTemplate)))
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/templates/{name}/preview", ready(s.handleBulkApplyTemplatePreview))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/workflows", ready(s.handleBulkApplyWorkflows))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/workflows/{name}", ready(s.handleBulkApplyWorkflow))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/workflows/{name}/runs", ready(s.handleBulkApplyWorkflowRuns))
	mux.HandleFunc("GET /api/vaults/{vault}/bulk-apply/runs/{run}", ready(s.handleBulkApplyRun))
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/workflows/{name}/precheck", s.requireTrustedIP(ready(s.handleBulkApplyWorkflowPrecheck)))
	mux.HandleFunc("POST /api/vaults/{vault}/bulk-apply/workflows/{name}/run", s.requireTrustedIP(ready(s.handleBulkApplyWorkflowRun)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys", ready(s.handleVaultKeys))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}", ready(s.handleVaultKeyGet))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/meta", ready(s.handleVaultKeyMeta))
	mux.HandleFunc("PATCH /api/vaults/{vault}/keys/{name}/meta", s.requireTrustedIP(ready(s.handleVaultKeyMetaPatch)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/summary", ready(s.handleVaultKeySummary))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/bindings", ready(s.handleVaultKeyBindings))
	mux.HandleFunc("POST /api/vaults/{vault}/keys/{name}/bindings", s.requireTrustedIP(ready(s.handleVaultKeyBindingSave)))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}/bindings", s.requireTrustedIP(ready(s.handleVaultKeyBindingsReplace)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}/bindings", s.requireTrustedIP(ready(s.handleVaultKeyBindingsDeleteAll)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/usage", ready(s.handleVaultKeyUsage))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/audit", ready(s.handleVaultKeyAudit))
	mux.HandleFunc("POST /api/vaults/{vault}/keys", s.requireTrustedIP(ready(s.handleVaultKeySave)))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}", s.requireTrustedIP(ready(s.handleVaultKeyUpdate)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/fields", ready(s.handleVaultKeyFields))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}/fields", s.requireTrustedIP(ready(s.handleVaultKeyFieldsUpdate)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/fields/{field}", ready(s.handleVaultKeyFieldGet))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}/fields/{field}", s.requireTrustedIP(ready(s.handleVaultKeyFieldUpdate)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}/fields/{field}", s.requireTrustedIP(ready(s.handleVaultKeyFieldDelete)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}/bindings/{binding_id}", s.requireTrustedIP(ready(s.handleVaultKeyBindingDelete)))
	mux.HandleFunc("POST /api/vaults/{vault}/keys/{name}/activate", s.requireTrustedIP(ready(s.handleVaultKeyActivate)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}", s.requireTrustedIP(ready(s.handleVaultKeyDelete)))
	mux.HandleFunc("POST /api/agents/{agent}/migrate", s.requireTrustedIP(ready(s.handleAgentMigrate)))
	mux.HandleFunc("POST /api/agents/{agent}/approve-rebind", s.requireTrustedIP(ready(s.handleAgentApproveRebind)))

	// Configs aggregate (cross-agent search/bulk-update)
	mux.HandleFunc("GET /api/configs/summary", ready(s.handleConfigsSummary))
	mux.HandleFunc("GET /api/configs/search/{key}", ready(s.handleConfigsSearch))
	mux.HandleFunc("POST /api/configs/bulk-update", s.requireTrustedIP(ready(s.handleConfigsBulkUpdate)))
	mux.HandleFunc("POST /api/configs/bulk-set", s.requireTrustedIP(ready(s.handleConfigsBulkSet)))

	// Agent configs (plaintext key-value proxy)
	mux.HandleFunc("GET /api/agents/{agent}/configs", ready(s.handleAgentConfigs))
	mux.HandleFunc("GET /api/agents/{agent}/configs/{key}", ready(s.handleAgentGetConfig))
	mux.HandleFunc("POST /api/agents/{agent}/configs", s.requireTrustedIP(ready(s.handleAgentSaveConfig)))
	mux.HandleFunc("PUT /api/agents/{agent}/configs/bulk", s.requireTrustedIP(ready(s.handleAgentSaveConfigsBulk)))
	mux.HandleFunc("DELETE /api/agents/{agent}/configs/{key}", s.requireTrustedIP(ready(s.handleAgentDeleteConfig)))
}

// getLocalDEK retrieves and decrypts the local node's DEK
func (s *Server) getLocalDEK() ([]byte, error) {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		return nil, fmt.Errorf("no node info: %w", err)
	}

	s.kekMu.RLock()
	kek := s.kek
	s.kekMu.RUnlock()

	dek, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}
	return dek, nil
}

// generateSecretRef generates a random hex ref of given length
func generateSecretRef(length int) (string, error) {
	bytes := make([]byte, (length+1)/2)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

// federatedSecretEntry represents a secret found on a child node
type federatedSecretEntry struct {
	NodeID  string `json:"node_id"`
	Label   string `json:"label"`
	URL     string `json:"url"`
	Name    string `json:"name"`
	Ref     string `json:"ref,omitempty"`
	Token   string `json:"token,omitempty"`
	Version int    `json:"version"`
	Value   string `json:"value,omitempty"`
}
