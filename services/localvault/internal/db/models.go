package db

import "time"

type NodeInfo struct {
	NodeID           string `gorm:"column:node_id;primaryKey"`
	DEK              []byte `gorm:"column:dek;not null"`
	DEKNonce         []byte `gorm:"column:dek_nonce;not null"`
	AgentSecret      []byte `gorm:"column:agent_secret"`
	AgentSecretNonce []byte `gorm:"column:agent_secret_nonce"`
	Version          int    `gorm:"column:version;default:1"`
}

func (NodeInfo) TableName() string { return "node_info" }

type Secret struct {
	ID             string     `gorm:"column:id;primaryKey"`
	Name           string     `gorm:"column:name;uniqueIndex;not null"`
	Ref            string     `gorm:"column:ref"`
	Ciphertext     []byte     `gorm:"column:ciphertext"`
	Nonce          []byte     `gorm:"column:nonce"`
	Version        int        `gorm:"column:version"`
	Scope          RefScope   `gorm:"column:scope;default:LOCAL"`
	Status         RefStatus  `gorm:"column:status;default:active"`
	Class          string     `gorm:"column:class;default:key"`
	DisplayName    string     `gorm:"column:display_name"`
	Description    string     `gorm:"column:description"`
	TagsJSON       string     `gorm:"column:tags_json;default:'[]'"`
	Origin         string     `gorm:"column:origin;default:sync"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	LastRotatedAt  *time.Time `gorm:"column:last_rotated_at"`
	LastRevealedAt *time.Time `gorm:"column:last_revealed_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (Secret) TableName() string { return "secrets" }

type SecretField struct {
	SecretName      string    `gorm:"column:secret_name;primaryKey"`
	FieldKey        string    `gorm:"column:field_key;primaryKey"`
	FieldType       string    `gorm:"column:field_type"`
	FieldRole       string    `gorm:"column:field_role"`
	DisplayName     string    `gorm:"column:display_name"`
	MaskedByDefault bool      `gorm:"column:masked_by_default"`
	Required        bool      `gorm:"column:required"`
	SortOrder       int       `gorm:"column:sort_order"`
	Ciphertext      []byte    `gorm:"column:ciphertext"`
	Nonce           []byte    `gorm:"column:nonce"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (SecretField) TableName() string { return "secret_fields" }

type Config struct {
	Key       string    `gorm:"column:key;primaryKey"`
	Value     string    `gorm:"column:value"`
	Scope     RefScope  `gorm:"column:scope;not null;default:LOCAL"`
	Status    RefStatus `gorm:"column:status;not null;default:active"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Config) TableName() string { return "configs" }

type Function struct {
	Name         string     `gorm:"column:name;primaryKey" json:"name"`
	Scope        string     `gorm:"column:scope;not null" json:"scope"`
	VaultHash    string     `gorm:"column:vault_hash;not null" json:"vault_hash"`
	FunctionHash string     `gorm:"column:function_hash;not null" json:"function_hash"`
	Category     string     `gorm:"column:category" json:"category"`
	Command      string     `gorm:"column:command;not null" json:"command"`
	VarsJSON     string     `gorm:"column:vars_json" json:"vars_json"`
	Description  string     `gorm:"column:description" json:"description"`
	TagsJSON     string     `gorm:"column:tags_json" json:"tags_json"`
	Provenance   string     `gorm:"column:provenance" json:"provenance"`
	LastTestedAt *time.Time `gorm:"column:last_tested_at" json:"last_tested_at"`
	LastRunAt    *time.Time `gorm:"column:last_run_at" json:"last_run_at"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Function) TableName() string { return "functions" }

type FunctionLog struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FunctionHash string    `gorm:"column:function_hash;not null" json:"function_hash"`
	Action       string    `gorm:"column:action;not null" json:"action"`
	Status       string    `gorm:"column:status;not null" json:"status"`
	DetailJSON   string    `gorm:"column:detail_json" json:"detail_json"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (FunctionLog) TableName() string { return "function_logs" }
