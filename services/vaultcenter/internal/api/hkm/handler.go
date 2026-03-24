package hkm

import (
	"context"
	"net/http"
	"time"

	"veilkey-vaultcenter/internal/db"

	chain "github.com/veilkey/veilkey-chain"
)

// Deps is the interface hkm.Handler uses to access server capabilities.
// *api.Server implements this; declared here to avoid circular import.
type Deps interface {
	// DB returns the underlying database handle.
	DB() *db.DB

	// HTTPClient returns the shared HTTP client.
	HTTPClient() *http.Client

	// GetKEK returns a thread-safe copy of the current KEK bytes.
	GetKEK() []byte

	// GetLocalDEK decrypts and returns the local node's DEK using the KEK.
	GetLocalDEK() ([]byte, error)

	// CascadeResolveTimeout is the timeout for federated resolve to children.
	CascadeResolveTimeout() time.Duration

	// ParentForwardTimeout is the timeout for forwarding to a parent node.
	ParentForwardTimeout() time.Duration

	// DeployTimeout is the timeout for deploying to children.
	DeployTimeout() time.Duration

	// IsTrustedIPString returns true if the given IP is trusted.
	IsTrustedIPString(ip string) bool

	// SubmitTx submits a write TX and blocks until committed.
	// Returns the result log (e.g. canonical ref) or error.
	SubmitTx(ctx context.Context, txType chain.TxType, payload any) (string, error)

	// SubmitTxAsync submits a write TX without waiting for block inclusion.
	// Used for high-frequency, loss-tolerant operations (heartbeat).
	SubmitTxAsync(ctx context.Context, txType chain.TxType, payload any) error

	// ChainInfo returns genesis JSON and persistent_peers for child nodes joining the chain.
	ChainInfo() (genesisJSON []byte, persistentPeers string)

	// MaskMapVersion returns the current mask_map version counter.
	MaskMapVersion() uint64

	// MaskMapWait returns a channel that closes when mask_map version changes.
	MaskMapWait() <-chan struct{}
}

// Handler owns all HKM HTTP handlers.
type Handler struct {
	deps Deps
}

// NewHandler creates an HKM Handler backed by the provided Deps.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Register mounts all HKM routes onto mux.
//   - requireTrustedIP  – restricts to trusted IP ranges
//   - requireReadyForOps – requires server unlocked + install complete
//   - requireAdminAuth  – requires admin session (password login)
func (h *Handler) Register(
	mux *http.ServeMux,
	requireTrustedIP func(http.HandlerFunc) http.HandlerFunc,
	requireReadyForOps func(http.HandlerFunc) http.HandlerFunc,
	requireAdminAuth func(http.HandlerFunc) http.HandlerFunc,
) {
	ready := requireReadyForOps
	trusted := requireTrustedIP
	adminAuth := requireAdminAuth

	// Parent API (called by root/parent to manage this node's children)
	mux.HandleFunc("POST /api/register", trusted(ready(h.handleRegister)))
	mux.HandleFunc("POST /api/rekey", trusted(ready(h.handleRekey)))
	mux.HandleFunc("GET /api/node-info", ready(h.handleNodeInfo))
	mux.HandleFunc("GET /api/children", ready(h.handleListChildren))
	mux.HandleFunc("DELETE /api/children/{node_id}", trusted(ready(h.handleDeleteChild)))
	mux.HandleFunc("GET /api/registry", ready(h.handleListRegistry))

	// Resolve scoped VK ref → plaintext value — requires admin session
	mux.HandleFunc("GET /api/resolve/{ref}", adminAuth(ready(h.handleResolveSecret)))

	// Child heartbeat (report URL)
	mux.HandleFunc("POST /api/heartbeat", trusted(ready(h.handleHeartbeat)))

	// Set parent URL (called on child to configure parent resolve)
	mux.HandleFunc("POST /api/set-parent", trusted(ready(h.handleSetParent)))

	// Key rotation (root triggers full tree rotation)
	mux.HandleFunc("POST /api/federation/rotate", trusted(ready(h.handleFederatedRotate)))

	// Mask map for veil-cli PTY masking — requires admin session
	mux.HandleFunc("GET /api/mask-map", adminAuth(trusted(ready(h.handleMaskMap))))

	// Agent management (Hub-only decryption)
	agentAuth := h.requireAgentAuth
	mux.HandleFunc("POST /api/agents/heartbeat", trusted(ready(h.handleAgentHeartbeat)))
	mux.HandleFunc("GET /api/agents/unlock-key", agentAuth(ready(h.handleAgentUnlockKey)))
	mux.HandleFunc("DELETE /api/agents/by-node/{node_id}", trusted(ready(h.handleAgentUnregisterByNode)))
	mux.HandleFunc("POST /api/agents/by-node/{node_id}/archive", trusted(ready(h.handleAgentArchive)))
	mux.HandleFunc("POST /api/agents/by-node/{node_id}/unarchive", trusted(ready(h.handleAgentUnarchive)))
	mux.HandleFunc("GET /api/resolve-agent/{token}", ready(h.handleAgentResolve))
	mux.HandleFunc("GET /api/agents", ready(h.handleAgentList))
	mux.HandleFunc("GET /api/agents/{agent}/rebind-plan", trusted(ready(h.handleAgentRebindPlan)))
	mux.HandleFunc("POST /api/agents/rotate-all", trusted(ready(h.handleAgentRotateAll)))
	mux.HandleFunc("GET /api/tracked-refs/audit", ready(h.handleTrackedRefAudit))
	mux.HandleFunc("POST /api/tracked-refs/cleanup", trusted(ready(h.handleTrackedRefCleanup)))
	mux.HandleFunc("POST /api/tracked-refs/sync", trusted(ready(h.handleTrackedRefSync)))
	mux.HandleFunc("GET /api/vault-inventory", ready(h.handleVaultInventory))
	mux.HandleFunc("GET /api/catalog/secrets", ready(h.handleSecretCatalogList))
	mux.HandleFunc("GET /api/catalog/secrets/{ref}", ready(h.handleSecretCatalogGet))
	mux.HandleFunc("GET /api/catalog/bindings", ready(h.handleBindingsList))
	mux.HandleFunc("GET /api/catalog/audit", ready(h.handleAuditEventsList))
	mux.HandleFunc("GET /api/ref-policy", ready(h.handleRefPolicy))
	mux.HandleFunc("GET /api/targets/{binding_type}/{target_name}/bindings", ready(h.handleTargetBindings))
	mux.HandleFunc("PUT /api/targets/{binding_type}/{target_name}/bindings", trusted(ready(h.handleTargetBindingsReplace)))
	mux.HandleFunc("DELETE /api/targets/{binding_type}/{target_name}/bindings", trusted(ready(h.handleTargetBindingsDeleteAll)))
	mux.HandleFunc("GET /api/targets/{binding_type}/{target_name}/impact", ready(h.handleTargetImpact))
	mux.HandleFunc("GET /api/targets/{binding_type}/{target_name}/summary", ready(h.handleTargetSummary))
	mux.HandleFunc("GET /api/functions/global", ready(h.handleGlobalFunctions))
	mux.HandleFunc("POST /api/functions/global", trusted(ready(h.handleGlobalFunctions)))
	mux.HandleFunc("GET /api/functions/global/{name}", ready(h.handleGlobalFunction))
	mux.HandleFunc("DELETE /api/functions/global/{name}", trusted(ready(h.handleGlobalFunction)))
	mux.HandleFunc("POST /api/functions/global/{name}/run", trusted(ready(h.handleGlobalFunctionRun)))
	mux.HandleFunc("GET /api/agents/{agent}/secrets", agentAuth(ready(h.handleAgentSecrets)))
	mux.HandleFunc("GET /api/agents/{agent}/secrets/{name}", agentAuth(ready(h.handleAgentGetSecret)))
	mux.HandleFunc("POST /api/agents/{agent}/secrets", agentAuth(trusted(ready(h.handleAgentSaveSecret))))
	mux.HandleFunc("POST /api/agents/{agent}/secrets/{name}/fields", agentAuth(trusted(ready(h.handleAgentSaveSecretFields))))
	mux.HandleFunc("GET /api/agents/{agent}/secrets/{name}/fields/{field}", agentAuth(ready(h.handleAgentGetSecretField)))
	mux.HandleFunc("DELETE /api/agents/{agent}/secrets/{name}/fields/{field}", agentAuth(trusted(ready(h.handleAgentDeleteSecretField))))
	mux.HandleFunc("DELETE /api/agents/{agent}/secrets/{name}", agentAuth(trusted(ready(h.handleAgentDeleteSecret))))
	mux.HandleFunc("GET /api/vaults", ready(h.handleVaultList))
	mux.HandleFunc("GET /api/vaults/{vault}", ready(h.handleVaultGet))
	mux.HandleFunc("PATCH /api/vaults/{vault}", trusted(ready(h.handleVaultPatch)))
	mux.HandleFunc("GET /api/vaults/{vault}/audit", ready(h.handleVaultAudit))
	mux.HandleFunc("GET /api/vaults/{vault}/keys", ready(h.handleVaultKeys))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}", ready(h.handleVaultKeyGet))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/meta", ready(h.handleVaultKeyMeta))
	mux.HandleFunc("PATCH /api/vaults/{vault}/keys/{name}/meta", trusted(ready(h.handleVaultKeyMetaPatch)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/summary", ready(h.handleVaultKeySummary))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/bindings", ready(h.handleVaultKeyBindings))
	mux.HandleFunc("POST /api/vaults/{vault}/keys/{name}/bindings", trusted(ready(h.handleVaultKeyBindingSave)))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}/bindings", trusted(ready(h.handleVaultKeyBindingsReplace)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}/bindings", trusted(ready(h.handleVaultKeyBindingsDeleteAll)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/usage", ready(h.handleVaultKeyUsage))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/audit", ready(h.handleVaultKeyAudit))
	mux.HandleFunc("POST /api/vaults/{vault}/keys", trusted(ready(h.handleVaultKeySave)))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}", trusted(ready(h.handleVaultKeyUpdate)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/fields", ready(h.handleVaultKeyFields))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}/fields", trusted(ready(h.handleVaultKeyFieldsUpdate)))
	mux.HandleFunc("GET /api/vaults/{vault}/keys/{name}/fields/{field}", ready(h.handleVaultKeyFieldGet))
	mux.HandleFunc("PUT /api/vaults/{vault}/keys/{name}/fields/{field}", trusted(ready(h.handleVaultKeyFieldUpdate)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}/fields/{field}", trusted(ready(h.handleVaultKeyFieldDelete)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}/bindings/{binding_id}", trusted(ready(h.handleVaultKeyBindingDelete)))
	mux.HandleFunc("POST /api/vaults/{vault}/keys/{name}/activate", trusted(ready(h.handleVaultKeyActivate)))
	mux.HandleFunc("DELETE /api/vaults/{vault}/keys/{name}", trusted(ready(h.handleVaultKeyDelete)))
	mux.HandleFunc("POST /api/agents/{agent}/migrate", agentAuth(trusted(ready(h.handleAgentMigrate))))
	mux.HandleFunc("POST /api/agents/{agent}/approve-rebind", trusted(ready(h.handleAgentApproveRebind)))

	// Configs aggregate (cross-agent search/bulk-update)
	mux.HandleFunc("GET /api/configs/summary", ready(h.handleConfigsSummary))
	mux.HandleFunc("GET /api/configs/search/{key}", ready(h.handleConfigsSearch))
	mux.HandleFunc("POST /api/configs/bulk-update", trusted(ready(h.handleConfigsBulkUpdate)))
	mux.HandleFunc("POST /api/configs/bulk-set", trusted(ready(h.handleConfigsBulkSet)))

	// Agent configs (plaintext key-value proxy)
	mux.HandleFunc("GET /api/agents/{agent}/configs", agentAuth(ready(h.handleAgentConfigs)))
	mux.HandleFunc("GET /api/agents/{agent}/configs/{key}", agentAuth(ready(h.handleAgentGetConfig)))
	mux.HandleFunc("POST /api/agents/{agent}/configs", agentAuth(trusted(ready(h.handleAgentSaveConfig))))
	mux.HandleFunc("PUT /api/agents/{agent}/configs/bulk", agentAuth(trusted(ready(h.handleAgentSaveConfigsBulk))))
	mux.HandleFunc("DELETE /api/agents/{agent}/configs/{key}", agentAuth(trusted(ready(h.handleAgentDeleteConfig))))

	// Identity aliases

	// Rotation progress
}
