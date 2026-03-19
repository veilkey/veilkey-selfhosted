package db

// DB config key constants — use these instead of hardcoded strings.
const (
	ConfigKeyVaultcenterPassword = "VAULTCENTER_PASSWORD"
	ConfigKeyAdminPassword       = "ADMIN_PASSWORD"
)

// DefaultAgentRole is the fallback role assigned to agents without an explicit role.
const DefaultAgentRole = "agent"

// MakeTemplateID builds a composite template ID from vault hash and template name.
func MakeTemplateID(vaultHash, name string) string {
	return vaultHash + ":" + name
}

// MakeSecretCanonicalID builds a composite secret canonical ID from vault hash and ref canonical.
func MakeSecretCanonicalID(vaultHash, refCanonical string) string {
	return vaultHash + ":" + refCanonical
}
