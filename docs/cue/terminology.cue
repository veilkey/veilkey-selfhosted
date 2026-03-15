package docs

// terminology.cue
// Canonical VeilKey identity terminology definitions.
// All services, configs, and documentation MUST use these exact field names.
// No plaintext secret values are included -- only typed placeholders.

// ---------------------------------------------------------------------------
// Identity primitive definitions
// ---------------------------------------------------------------------------

#VaultNodeUUID: {
	// The UUID that uniquely identifies a vault node within a fleet.
	// Format: standard UUID v4.
	field_name:  "vault_node_uuid"
	type:        "string (uuid)"
	description: "Unique identifier assigned to each vault node at provisioning time."
	example:     "VK:REF:vault-node-uuid-placeholder"
}

#VaultHash: {
	// Content-addressed hash of the sealed vault state on disk.
	field_name:  "vault_hash"
	type:        "string (hex digest)"
	description: "Integrity hash of the sealed vault state. Used for tamper detection."
	example:     "VK:REF:vault-hash-placeholder"
}

#VaultRuntimeHash: {
	// Hash capturing the integrity of the running vault process and its
	// loaded configuration at a specific point in time.
	field_name:  "vault_runtime_hash"
	type:        "string (hex digest)"
	description: "Runtime integrity hash of the active vault process. Verified during attestation."
	example:     "VK:REF:vault-runtime-hash-placeholder"
}

// ---------------------------------------------------------------------------
// Collected terminology catalog
// ---------------------------------------------------------------------------

#TerminologyCatalog: {
	vault_node_uuid:    #VaultNodeUUID
	vault_hash:         #VaultHash
	vault_runtime_hash: #VaultRuntimeHash
}

terminology_catalog: #TerminologyCatalog & {
	vault_node_uuid:    #VaultNodeUUID
	vault_hash:         #VaultHash
	vault_runtime_hash: #VaultRuntimeHash
}
