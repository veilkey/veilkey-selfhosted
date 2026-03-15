package docs

// hardcoding_contract.cue
// Contract for hardcoding governance across the veilkey-selfhosted monorepo.
// Defines what fields must never contain literal values and which patterns
// are blocked in configuration and source files.

// ---------------------------------------------------------------------------
// Governance modes
// ---------------------------------------------------------------------------

#GovernanceMode: "audit" | "blocking"

// "audit"    -- violations are logged as warnings; pipeline continues.
// "blocking" -- violations cause the pipeline to fail immediately.

governance: {
	// Default mode for new checks. Operators may override per-environment.
	default_mode: #GovernanceMode & "blocking"

	// When true, CI prints every matched violation even in blocking mode
	// before failing, so operators get a full report.
	report_all_before_fail: bool | *true
}

// ---------------------------------------------------------------------------
// Blocked patterns
// ---------------------------------------------------------------------------
// Each entry describes a category of literal values that must not appear
// in committed configuration, CUE schemas, docker-compose files, .env
// templates, or installer scripts.

#BlockedPattern: {
	id:          string
	description: string
	// A regex (PCRE-style) that scanners should apply.
	regex:       string
	mode:        #GovernanceMode
	// File globs where this rule applies. Empty means all files.
	applies_to: [...string]
}

blocked_patterns: [...#BlockedPattern] & [
	{
		id:          "plaintext_secret"
		description: "Plaintext secrets or API keys must never be committed."
		regex:       "(PRIVATE_KEY|SECRET_KEY|API_KEY|PASSWORD|TOKEN)\\s*[:=]\\s*[\"'][^\"']{8,}[\"']"
		mode:        "blocking"
		applies_to: []
	},
	{
		id:          "hardcoded_ip"
		description: "Production IP addresses must not be hardcoded; use DNS or service discovery."
		regex:        "\\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\b"
		mode:        "audit"
		applies_to: [
			"docker-compose*.yml",
			"*.env",
			"*.env.example",
			"installer/**",
		]
	},
	{
		id:          "hardcoded_port"
		description: "Production ports must be sourced from configuration, not hardcoded literals."
		regex:       "port\\s*[:=]\\s*[0-9]{2,5}"
		mode:        "audit"
		applies_to: [
			"docker-compose*.yml",
			"installer/**",
		]
	},
	{
		id:          "embedded_credential"
		description: "Embedded usernames or passwords in connection strings are forbidden."
		regex:       "://[a-zA-Z0-9_]+:[^@]+@"
		mode:        "blocking"
		applies_to: []
	},
	{
		id:          "vault_node_uuid_literal"
		description: "vault_node_uuid must be injected at runtime, never hardcoded."
		regex:       "vault_node_uuid\\s*[:=]\\s*\"[0-9a-fA-F-]{36}\""
		mode:        "blocking"
		applies_to: []
	},
	{
		id:          "vault_hash_literal"
		description: "vault_hash must be computed, never hardcoded."
		regex:       "vault_hash\\s*[:=]\\s*\"[0-9a-fA-F]{64}\""
		mode:        "blocking"
		applies_to: []
	},
	{
		id:          "vault_runtime_hash_literal"
		description: "vault_runtime_hash must be computed, never hardcoded."
		regex:       "vault_runtime_hash\\s*[:=]\\s*\"[0-9a-fA-F]{64}\""
		mode:        "blocking"
		applies_to: []
	},
]

// ---------------------------------------------------------------------------
// Never-hardcode fields
// ---------------------------------------------------------------------------
// These field names must always resolve via environment variable, secret
// manager reference, or runtime injection -- never a literal in source.

#NeverHardcodeField: {
	field:       string
	reason:      string
	inject_via:  "env" | "secret_manager" | "runtime" | "config_ref"
}

never_hardcode: [...#NeverHardcodeField] & [
	{
		field:      "vault_node_uuid"
		reason:     "Unique per node; must come from provisioning."
		inject_via: "runtime"
	},
	{
		field:      "vault_hash"
		reason:     "Integrity value computed at seal time."
		inject_via: "runtime"
	},
	{
		field:      "vault_runtime_hash"
		reason:     "Integrity value computed at process start."
		inject_via: "runtime"
	},
	{
		field:      "tls_private_key"
		reason:     "Private key material must never appear in source."
		inject_via: "secret_manager"
	},
	{
		field:      "database_password"
		reason:     "Credentials must be injected, not stored in config."
		inject_via: "secret_manager"
	},
	{
		field:      "api_token"
		reason:     "Tokens must be injected per-environment."
		inject_via: "env"
	},
]

// ---------------------------------------------------------------------------
// Allowlist for known-safe literals
// ---------------------------------------------------------------------------
// Some values look like violations but are intentional (e.g. localhost in
// dev-only docker-compose, documentation examples).

#Allowlist: {
	pattern:     string
	reason:      string
	files:       [...string]
}

allowlist: [...#Allowlist] & [
	{
		pattern: "127.0.0.1"
		reason:  "Loopback address acceptable in local development configs."
		files: [
			"docker-compose.dev.yml",
			"docker-compose.override.yml",
		]
	},
	{
		pattern: "VK:REF:example"
		reason:  "Placeholder used in documentation and CUE contracts."
		files: [
			"docs/**",
			"facts/**",
		]
	},
]
