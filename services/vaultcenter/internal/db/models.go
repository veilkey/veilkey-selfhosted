package db

import (
	"time"
)

type EncryptionKey struct {
	Version      int        `gorm:"primaryKey;autoIncrement;column:version" json:"version"`
	EncryptedDEK []byte     `gorm:"column:encrypted_dek;not null" json:"encrypted_dek"`
	Nonce        []byte     `gorm:"column:nonce;not null" json:"nonce"`
	Algorithm    string     `gorm:"column:algorithm;default:AES-256-GCM" json:"algorithm"`
	Status       string     `gorm:"column:status;default:active" json:"status"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	RetiredAt    *time.Time `gorm:"column:retired_at" json:"retired_at"`
}

func (EncryptionKey) TableName() string { return "encryption_keys" }

type TokenRef struct {
	RefCanonical  string     `gorm:"primaryKey;column:ref_canonical;size:96" json:"ref_canonical"`
	RefFamily     string     `gorm:"column:ref_family;size:16;not null;index:idx_token_refs_family_scope" json:"ref_family"`
	RefScope      RefScope   `gorm:"column:ref_scope;size:16;not null;index:idx_token_refs_family_scope" json:"ref_scope"`
	RefID         string     `gorm:"column:ref_id;size:64;not null;index" json:"ref_id"`
	SecretName    string     `gorm:"column:secret_name;size:255;not null;default:'';index" json:"secret_name"`
	AgentHash     string     `gorm:"column:agent_hash;size:16;index" json:"agent_hash"`
	PlaintextHash string     `gorm:"column:plaintext_hash;size:64;index" json:"plaintext_hash,omitempty"`
	Ciphertext    string     `gorm:"column:ciphertext;not null" json:"ciphertext"`
	Version       int        `gorm:"column:version;not null" json:"version"`
	Status        RefStatus  `gorm:"column:status;default:temp" json:"status"`
	ExpiresAt     *time.Time `gorm:"column:expires_at;index" json:"expires_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (TokenRef) TableName() string { return "token_refs" }

type NodeInfo struct {
	NodeID         string    `gorm:"primaryKey;column:node_id" json:"node_id"`
	ParentURL      string    `gorm:"column:parent_url" json:"parent_url"`
	DEK            []byte    `gorm:"column:dek;not null" json:"dek"`
	DEKNonce       []byte    `gorm:"column:dek_nonce;not null" json:"dek_nonce"`
	DEKMaster      []byte    `gorm:"column:dek_master" json:"dek_master"`
	DEKMasterNonce []byte    `gorm:"column:dek_master_nonce" json:"dek_master_nonce"`
	Version        int       `gorm:"column:version;default:1" json:"version"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (NodeInfo) TableName() string { return "node_info" }

type Child struct {
	NodeID       string     `gorm:"primaryKey;column:node_id" json:"node_id"`
	Label        string     `gorm:"column:label" json:"label"`
	URL          string     `gorm:"column:url" json:"url"`
	EncryptedDEK []byte     `gorm:"column:encrypted_dek;not null" json:"encrypted_dek"`
	Nonce        []byte     `gorm:"column:nonce;not null" json:"nonce"`
	Version      int        `gorm:"column:version;default:1" json:"version"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastSeen     *time.Time `gorm:"column:last_seen" json:"last_seen"`
}

func (Child) TableName() string { return "children" }

type Secret struct {
	ID         string    `gorm:"primaryKey;column:id" json:"id"`
	Name       string    `gorm:"column:name;uniqueIndex;not null" json:"name"`
	Ref        string    `gorm:"column:ref;uniqueIndex:idx_secrets_ref" json:"ref"`
	Ciphertext []byte    `gorm:"column:ciphertext;not null" json:"ciphertext"`
	Nonce      []byte    `gorm:"column:nonce;not null" json:"nonce"`
	Version    int       `gorm:"column:version;not null" json:"version"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Secret) TableName() string { return "secrets" }

type VaultInventory struct {
	VaultNodeUUID    string     `gorm:"primaryKey;column:vault_node_uuid" json:"vault_node_uuid"`
	VaultRuntimeHash string     `gorm:"column:vault_runtime_hash;not null;uniqueIndex" json:"vault_runtime_hash"`
	VaultHash        string     `gorm:"column:vault_hash;not null;index" json:"vault_hash"`
	VaultName        string     `gorm:"column:vault_name;not null;index" json:"vault_name"`
	DisplayName      string     `gorm:"column:display_name;not null;default:''" json:"display_name"`
	Description      string     `gorm:"column:description;not null;default:''" json:"description"`
	TagsJSON         string     `gorm:"column:tags_json;not null;default:'[]'" json:"tags_json"`
	ManagedPathsJSON string     `gorm:"column:managed_paths_json;not null;default:'[]'" json:"managed_paths_json"`
	Mode             string     `gorm:"column:mode;not null;default:localvault" json:"mode"`
	CapabilitiesJSON string     `gorm:"column:capabilities_json;not null;default:'[]'" json:"capabilities_json"`
	Status           string     `gorm:"column:status;not null;default:ok;index" json:"status"`
	Blocked          bool       `gorm:"column:blocked;not null;default:false" json:"blocked"`
	RotationRequired bool       `gorm:"column:rotation_required;not null;default:false" json:"rotation_required"`
	RebindRequired   bool       `gorm:"column:rebind_required;not null;default:false" json:"rebind_required"`
	FirstSeenAt      time.Time  `gorm:"column:first_seen_at;autoCreateTime" json:"first_seen_at"`
	LastSeenAt       time.Time  `gorm:"column:last_seen_at;autoUpdateTime" json:"last_seen_at"`
	RenamedAt        *time.Time `gorm:"column:renamed_at" json:"renamed_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (VaultInventory) TableName() string { return "vault_inventory" }

type SecretCatalog struct {
	SecretCanonicalID string     `gorm:"primaryKey;column:secret_canonical_id" json:"secret_canonical_id"`
	SecretName        string     `gorm:"column:secret_name;not null;index" json:"secret_name"`
	DisplayName       string     `gorm:"column:display_name;not null;default:''" json:"display_name"`
	Description       string     `gorm:"column:description;not null;default:''" json:"description"`
	TagsJSON          string     `gorm:"column:tags_json;not null;default:'[]'" json:"tags_json"`
	Class             string     `gorm:"column:class;not null;default:key;index" json:"class"`
	Scope             RefScope   `gorm:"column:scope;not null" json:"scope"`
	Status            RefStatus  `gorm:"column:status;not null;index" json:"status"`
	VaultNodeUUID     string     `gorm:"column:vault_node_uuid;not null;index" json:"vault_node_uuid"`
	VaultRuntimeHash  string     `gorm:"column:vault_runtime_hash;not null" json:"vault_runtime_hash"`
	VaultHash         string     `gorm:"column:vault_hash;not null;index" json:"vault_hash"`
	RefCanonical      string     `gorm:"column:ref_canonical;not null;uniqueIndex" json:"ref_canonical"`
	FieldsPresentJSON string     `gorm:"column:fields_present_json;not null;default:'[]'" json:"fields_present_json"`
	BindingCount      int        `gorm:"column:binding_count;not null;default:0" json:"binding_count"`
	LastRotatedAt     *time.Time `gorm:"column:last_rotated_at" json:"last_rotated_at"`
	LastRevealedAt    *time.Time `gorm:"column:last_revealed_at" json:"last_revealed_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (SecretCatalog) TableName() string { return "secret_catalog" }

type Binding struct {
	BindingID    string    `gorm:"primaryKey;column:binding_id" json:"binding_id"`
	BindingType  string    `gorm:"column:binding_type;not null;index:idx_bindings_target" json:"binding_type"`
	TargetName   string    `gorm:"column:target_name;not null;index:idx_bindings_target" json:"target_name"`
	VaultHash    string    `gorm:"column:vault_hash;not null" json:"vault_hash"`
	SecretName   string    `gorm:"column:secret_name;not null;index:idx_bindings_secret" json:"secret_name"`
	FieldKey     string    `gorm:"column:field_key;not null;default:'';index:idx_bindings_secret" json:"field_key"`
	RefCanonical string    `gorm:"column:ref_canonical;not null;index" json:"ref_canonical"`
	Required     bool      `gorm:"column:required;not null;default:true" json:"required"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Binding) TableName() string { return "bindings" }

type AuditEvent struct {
	EventID             string    `gorm:"primaryKey;column:event_id" json:"event_id"`
	EntityType          string    `gorm:"column:entity_type;not null;index:idx_audit_events_entity" json:"entity_type"`
	EntityID            string    `gorm:"column:entity_id;not null;index:idx_audit_events_entity" json:"entity_id"`
	Action              string    `gorm:"column:action;not null;index" json:"action"`
	ActorType           string    `gorm:"column:actor_type;not null;default:system" json:"actor_type"`
	ActorID             string    `gorm:"column:actor_id;not null;default:''" json:"actor_id"`
	Reason              string    `gorm:"column:reason;not null;default:''" json:"reason"`
	Source              string    `gorm:"column:source;not null;default:''" json:"source"`
	ApprovalChallengeID string    `gorm:"column:approval_challenge_id;not null;default:''" json:"approval_challenge_id"`
	BeforeJSON          string    `gorm:"column:before_json;not null;default:'{}'" json:"before_json"`
	AfterJSON           string    `gorm:"column:after_json;not null;default:'{}'" json:"after_json"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime;index" json:"created_at"`
}

func (AuditEvent) TableName() string { return "audit_events" }

type KeyRegistryEntry struct {
	NodeID   string    `gorm:"primaryKey;column:node_id" json:"node_id"`
	SecretID string    `gorm:"primaryKey;column:secret_id" json:"secret_id"`
	KeyName  string    `gorm:"column:key_name;not null;index:idx_registry_key_name" json:"key_name"`
	Version  int       `gorm:"column:version;not null" json:"version"`
	SyncedAt time.Time `gorm:"column:synced_at;autoCreateTime" json:"synced_at"`
}

func (KeyRegistryEntry) TableName() string { return "key_registry" }

type Agent struct {
	NodeID           string     `gorm:"primaryKey;column:node_id" json:"node_id"`
	Label            string     `gorm:"column:label;not null" json:"label"`
	AgentHash        string     `gorm:"column:agent_hash;uniqueIndex" json:"agent_hash"`
	VaultHash        string     `gorm:"column:vault_hash;index" json:"vault_hash"`
	VaultName        string     `gorm:"column:vault_name" json:"vault_name"`
	AgentRole        string     `gorm:"column:agent_role;not null;default:agent;index" json:"agent_role"`
	HostEnabled      bool       `gorm:"column:host_enabled;not null;default:false" json:"host_enabled"`
	LocalEnabled     bool       `gorm:"column:local_enabled;not null;default:true" json:"local_enabled"`
	ManagedPaths     string     `gorm:"column:managed_paths;type:text" json:"managed_paths"`
	KeyVersion       int        `gorm:"column:key_version;default:1" json:"key_version"`
	RotationRequired bool       `gorm:"column:rotation_required;default:false" json:"rotation_required"`
	RotationReason   string     `gorm:"column:rotation_reason" json:"rotation_reason"`
	RebindRequired   bool       `gorm:"column:rebind_required;default:false" json:"rebind_required"`
	RebindReason     string     `gorm:"column:rebind_reason" json:"rebind_reason"`
	RetryStage       int        `gorm:"column:retry_stage;default:0" json:"retry_stage"`
	NextRetryAt      *time.Time `gorm:"column:next_retry_at" json:"next_retry_at"`
	BlockedAt        *time.Time `gorm:"column:blocked_at" json:"blocked_at"`
	BlockReason      string     `gorm:"column:block_reason" json:"block_reason"`
	ArchivedAt       *time.Time `gorm:"column:archived_at;index" json:"archived_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at;index" json:"deleted_at"`
	IP               string     `gorm:"column:ip" json:"ip"`
	Port             int        `gorm:"column:port;default:0" json:"port"`
	DEK              []byte     `gorm:"column:dek" json:"dek"`
	DEKNonce         []byte     `gorm:"column:dek_nonce" json:"dek_nonce"`
	SecretsCount     int        `gorm:"column:secrets_count;default:0" json:"secrets_count"`
	ConfigsCount     int        `gorm:"column:configs_count;default:0" json:"configs_count"`
	AgentSecretHash    string     `gorm:"column:agent_secret_hash;size:64" json:"agent_secret_hash,omitempty"`
	AgentSecretEnc     []byte     `gorm:"column:agent_secret_enc" json:"-"`
	AgentSecretNonce   []byte     `gorm:"column:agent_secret_nonce" json:"-"`
	VaultUnlockKeyEnc  []byte     `gorm:"column:vault_unlock_key_enc" json:"-"`
	VaultUnlockKeyNonce []byte    `gorm:"column:vault_unlock_key_nonce" json:"-"`
	Salt             string     `gorm:"column:salt" json:"-"`
	Version          int        `gorm:"column:version;default:1" json:"version"`
	FirstSeen        time.Time  `gorm:"column:first_seen;autoCreateTime" json:"first_seen"`
	LastSeen         time.Time  `gorm:"column:last_seen;autoUpdateTime" json:"last_seen"`
}

func (Agent) TableName() string { return "agents" }

type GlobalFunction struct {
	Name         string    `gorm:"primaryKey;column:name" json:"name"`
	FunctionHash string    `gorm:"column:function_hash;uniqueIndex;not null" json:"function_hash"`
	Category     string    `gorm:"column:category;not null;default:''" json:"category"`
	Command      string    `gorm:"column:command;not null" json:"command"`
	VarsJSON     string    `gorm:"column:vars_json;type:text;not null" json:"vars_json"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (GlobalFunction) TableName() string { return "global_functions" }

type SecretInputChallenge struct {
	Token      string     `gorm:"primaryKey;column:token" json:"token"`
	Email      string     `gorm:"column:email;not null" json:"email"`
	Endpoint   string     `gorm:"column:endpoint;not null" json:"endpoint"`
	Vault      string     `gorm:"column:vault;not null" json:"vault"`
	SecretName string     `gorm:"column:secret_name;not null" json:"secret_name"`
	Reason     string     `gorm:"column:reason;not null;default:''" json:"reason"`
	Status     string     `gorm:"column:status;not null;default:pending;index" json:"status"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	UsedAt     *time.Time `gorm:"column:used_at" json:"used_at"`
}

func (SecretInputChallenge) TableName() string { return "secret_input_challenges" }

type EmailOTPChallenge struct {
	Token         string     `gorm:"primaryKey;column:token" json:"token"`
	Email         string     `gorm:"column:email;not null" json:"email"`
	Reason        string     `gorm:"column:reason;not null;default:''" json:"reason"`
	Status        string     `gorm:"column:status;not null;default:pending;index" json:"status"`
	CodeHash      string     `gorm:"column:code_hash;not null;default:''" json:"code_hash"`
	CodeExpiresAt *time.Time `gorm:"column:code_expires_at" json:"code_expires_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	UsedAt        *time.Time `gorm:"column:used_at" json:"used_at"`
}

func (EmailOTPChallenge) TableName() string { return "email_otp_challenges" }

type ApprovalTokenChallenge struct {
	Token       string     `gorm:"primaryKey;column:token" json:"token"`
	Kind        string     `gorm:"column:kind;not null;index" json:"kind"`
	Title       string     `gorm:"column:title;not null;default:''" json:"title"`
	Prompt      string     `gorm:"column:prompt;not null;default:''" json:"prompt"`
	InputLabel  string     `gorm:"column:input_label;not null;default:''" json:"input_label"`
	SubmitLabel string     `gorm:"column:submit_label;not null;default:''" json:"submit_label"`
	TargetName  string     `gorm:"column:target_name;not null;default:''" json:"target_name"`
	Status      string     `gorm:"column:status;not null;default:pending;index" json:"status"`
	Ciphertext  []byte     `gorm:"column:ciphertext" json:"ciphertext"`
	Nonce       []byte     `gorm:"column:nonce" json:"nonce"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	UsedAt      *time.Time `gorm:"column:used_at" json:"used_at"`
}

func (ApprovalTokenChallenge) TableName() string { return "approval_token_challenges" }

type AdminAuthConfig struct {
	ConfigID                string     `gorm:"primaryKey;column:config_id" json:"config_id"`
	PasswordHash            string     `gorm:"column:password_hash;not null;default:''" json:"password_hash"`
	TOTPEnabled             bool       `gorm:"column:totp_enabled;not null;default:false" json:"totp_enabled"`
	EnrolledAt              *time.Time `gorm:"column:enrolled_at" json:"enrolled_at"`
	TOTPSecretCiphertext    []byte     `gorm:"column:totp_secret_ciphertext" json:"totp_secret_ciphertext"`
	TOTPSecretNonce         []byte     `gorm:"column:totp_secret_nonce" json:"totp_secret_nonce"`
	PendingSecretCiphertext []byte     `gorm:"column:pending_secret_ciphertext" json:"pending_secret_ciphertext"`
	PendingSecretNonce      []byte     `gorm:"column:pending_secret_nonce" json:"pending_secret_nonce"`
	PendingIssuedAt         *time.Time `gorm:"column:pending_issued_at" json:"pending_issued_at"`
	UpdatedAt               time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (AdminAuthConfig) TableName() string { return "admin_auth_configs" }

type AdminSession struct {
	SessionID     string     `gorm:"primaryKey;column:session_id" json:"session_id"`
	TokenHash     string     `gorm:"column:token_hash;not null;uniqueIndex" json:"token_hash"`
	AuthMethod    string     `gorm:"column:auth_method;not null;default:totp" json:"auth_method"`
	RemoteAddr    string     `gorm:"column:remote_addr;not null;default:''" json:"remote_addr"`
	RevealUntil   *time.Time `gorm:"column:reveal_until" json:"reveal_until"`
	ExpiresAt     time.Time  `gorm:"column:expires_at;not null;index" json:"expires_at"`
	IdleExpiresAt time.Time  `gorm:"column:idle_expires_at;not null;index" json:"idle_expires_at"`
	LastSeenAt    time.Time  `gorm:"column:last_seen_at;not null;index" json:"last_seen_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	RevokedAt     *time.Time `gorm:"column:revoked_at;index" json:"revoked_at"`
}

func (AdminSession) TableName() string { return "admin_sessions" }

type UIConfig struct {
	ConfigID       string    `gorm:"primaryKey;column:config_id" json:"config_id"`
	Locale         string    `gorm:"column:locale;not null;default:ko" json:"locale"`
	DefaultEmail   string    `gorm:"column:default_email;not null;default:''" json:"default_email"`
	TargetVersion  string    `gorm:"column:target_version;not null;default:''" json:"target_version"`
	ReleaseChannel string    `gorm:"column:release_channel;not null;default:stable" json:"release_channel"`
	PublicBaseURL  string    `gorm:"column:public_base_url;not null;default:''" json:"public_base_url"`
	InstallProfile string    `gorm:"column:install_profile;not null;default:''" json:"install_profile"`
	InstallRoot    string    `gorm:"column:install_root;not null;default:''" json:"install_root"`
	InstallScript  string    `gorm:"column:install_script;not null;default:''" json:"install_script"`
	InstallWorkdir string    `gorm:"column:install_workdir;not null;default:''" json:"install_workdir"`
	VaultcenterURL string    `gorm:"column:vaultcenter_url;not null;default:''" json:"vaultcenter_url"`
	LocalvaultURL  string    `gorm:"column:localvault_url;not null;default:''" json:"localvault_url"`
	TLSCertPath    string    `gorm:"column:tls_cert_path;not null;default:''" json:"tls_cert_path"`
	TLSKeyPath     string    `gorm:"column:tls_key_path;not null;default:''" json:"tls_key_path"`
	TLSCAPath      string    `gorm:"column:tls_ca_path;not null;default:''" json:"tls_ca_path"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (UIConfig) TableName() string { return "ui_configs" }

type Config struct {
	Key       string    `gorm:"primaryKey;column:key" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null;default:''" json:"value"`
	Scope     RefScope  `gorm:"column:scope;not null;default:LOCAL" json:"scope"`
	Status    RefStatus `gorm:"column:status;not null;default:active" json:"status"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Config) TableName() string { return "configs" }

type BulkApplyTemplate struct {
	TemplateID       string    `gorm:"primaryKey;column:template_id" json:"template_id"`
	VaultRuntimeHash string    `gorm:"column:vault_runtime_hash;not null;index:idx_bulk_apply_templates_vault_name,priority:1" json:"vault_runtime_hash"`
	Name             string    `gorm:"column:name;not null;index:idx_bulk_apply_templates_vault_name,priority:2" json:"name"`
	Format           string    `gorm:"column:format;not null;default:env" json:"format"`
	TargetPath       string    `gorm:"column:target_path;not null;default:''" json:"target_path"`
	Body             string    `gorm:"column:body;type:text;not null;default:''" json:"body"`
	Hook             string    `gorm:"column:hook;not null;default:''" json:"hook"`
	Enabled          bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (BulkApplyTemplate) TableName() string { return "bulk_apply_templates" }

type BulkApplyRun struct {
	RunID            string    `gorm:"primaryKey;column:run_id" json:"run_id"`
	VaultRuntimeHash string    `gorm:"column:vault_runtime_hash;not null;index:idx_bulk_apply_runs_vault_workflow_created,priority:1" json:"vault_runtime_hash"`
	WorkflowName     string    `gorm:"column:workflow_name;not null;index:idx_bulk_apply_runs_vault_workflow_created,priority:2" json:"workflow_name"`
	RunKind          string    `gorm:"column:run_kind;not null;index" json:"run_kind"`
	Status           string    `gorm:"column:status;not null;index" json:"status"`
	SummaryJSON      string    `gorm:"column:summary_json;type:text;not null;default:'{}'" json:"summary_json"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime;index:idx_bulk_apply_runs_vault_workflow_created,priority:3,sort:desc" json:"created_at"`
}

func (BulkApplyRun) TableName() string { return "bulk_apply_runs" }

type Migration struct {
	Version   int       `gorm:"primaryKey;column:version" json:"version"`
	AppliedAt time.Time `gorm:"column:applied_at;autoCreateTime" json:"applied_at"`
}

func (Migration) TableName() string { return "migrations" }

type RegistrationToken struct {
	TokenID    string     `gorm:"primaryKey;column:token_id" json:"token_id"`
	Label      string     `gorm:"column:label;not null;default:''" json:"label"`
	CreatedBy  string     `gorm:"column:created_by;not null;default:admin" json:"created_by"`
	Status     string     `gorm:"column:status;not null;default:active;index" json:"status"` // active, used, revoked, expired
	UsedByNode string     `gorm:"column:used_by_node;not null;default:''" json:"used_by_node"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null;index" json:"expires_at"`
	UsedAt     *time.Time `gorm:"column:used_at" json:"used_at"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (RegistrationToken) TableName() string { return "registration_tokens" }

type AdminPasskey struct {
	CredentialID string    `gorm:"primaryKey;column:credential_id" json:"credential_id"`
	Name         string    `gorm:"column:name;not null" json:"name"`
	PublicKey    []byte    `gorm:"column:public_key;not null" json:"-"`
	AAGUID       string    `gorm:"column:aaguid;not null;default:''" json:"aaguid"`
	SignCount    uint32    `gorm:"column:sign_count;not null;default:0" json:"sign_count"`
	Transports   string    `gorm:"column:transports;not null;default:''" json:"transports"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (AdminPasskey) TableName() string { return "admin_passkeys" }

type SSHKey struct {
	Ref             string    `gorm:"primaryKey;column:ref" json:"ref"`
	Ownership       string    `gorm:"column:ownership;not null;default:'own'" json:"ownership"` // own | external
	Label           string    `gorm:"column:label" json:"label"`
	KeyType         string    `gorm:"column:key_type" json:"key_type"` // ed25519 | rsa | ecdsa
	Fingerprint     string    `gorm:"column:fingerprint;uniqueIndex" json:"fingerprint"`
	PrivateKeyEnc   []byte    `gorm:"column:private_key_enc" json:"-"`
	PrivateKeyNonce []byte    `gorm:"column:private_key_nonce" json:"-"`
	PublicKey       string    `gorm:"column:public_key;not null" json:"public_key"`
	HostsJSON       string    `gorm:"column:hosts_json;type:text;default:'[]'" json:"hosts_json"`
	MetadataJSON    string    `gorm:"column:metadata_json;type:text;default:'{}'" json:"metadata_json"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (SSHKey) TableName() string { return "ssh_keys" }
