package docs

// docs_contract.cue
// Contract for the docs directory structure and canonical VeilKey terminology.
// This schema is authoritative for documentation layout and naming conventions
// across the veilkey-selfhosted monorepo.

import "strings"

// version must match the content of the VERSION file (trimmed).
// During CI or manual audit, compare this against $(cat VERSION).
#Version: =~"^[0-9]+\\.[0-9]+\\.[0-9]+(-[a-zA-Z0-9.]+)?(\\+[a-zA-Z0-9.]+)?$"

version: #Version & "0.1.0"

// ---------------------------------------------------------------------------
// Directory structure contract
// ---------------------------------------------------------------------------

#DocsStructure: {
	// Required top-level files that must exist in the repository root.
	required_root_files: ["README.md", "VERSION", "VERSIONING.md"]

	// Required service-level documentation.
	required_service_docs: {
		keycenter:  "services/keycenter/README.md"
		localvault: "services/localvault/README.md"
	}

	// Subdirectories that the docs tree may contain.
	// "cue" is always required; others are optional.
	subdirs: {
		cue: required: true
		[string]: required: bool
	}
}

docs_structure: #DocsStructure & {
	subdirs: {
		cue: required: true
	}
}

// ---------------------------------------------------------------------------
// Canonical VeilKey terminology
// ---------------------------------------------------------------------------

// All schemas, configs, and documentation MUST use these canonical field
// names when referring to VeilKey identity and integrity primitives.
// Using alternative spellings (e.g. "node_uuid", "nodeUUID", "runtimeHash")
// is a contract violation.

#CanonicalTerminology: {
	// The UUID that uniquely identifies a vault node in a fleet.
	vault_node_uuid: string

	// The content-addressed hash of the sealed vault state.
	vault_hash: string

	// The hash capturing the running vault process integrity.
	vault_runtime_hash: string
}

// Reference example (no real values -- placeholder only).
_terminology_example: #CanonicalTerminology & {
	vault_node_uuid:    "VK:REF:example-uuid"
	vault_hash:         "VK:REF:example-vault-hash"
	vault_runtime_hash: "VK:REF:example-runtime-hash"
}

// ---------------------------------------------------------------------------
// Blocked alternative spellings
// ---------------------------------------------------------------------------
// These patterns must NOT appear as field names in any CUE or config file.
// CI tooling should scan for them and flag violations.

#BlockedTerms: [...string]

blocked_terms: #BlockedTerms & [
	"node_uuid",
	"nodeUUID",
	"nodeId",
	"node_id",
	"runtimeHash",
	"runtime_hash",
	"vaultHash",
]

// ---------------------------------------------------------------------------
// Version cross-check helper
// ---------------------------------------------------------------------------
// When evaluated, `version` above must equal the trimmed content of VERSION.
// A CI step can run:
//   cue eval docs/cue/docs_contract.cue -e version
// and compare with $(cat VERSION).

_version_note: strings.TrimSpace(version)
