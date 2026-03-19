package configs

import "veilkey-localvault/internal/db"

// Package-local aliases for db ref constants — keeps handler code concise.
const (
	refFamilyVE = db.RefFamilyVE

	refScopeLocal = db.RefScopeLocal

	refStatusActive = db.RefStatusActive
	refStatusBlock  = db.RefStatusBlock
)

// makeRef constructs a canonical ref string from its components.
func makeRef(family string, scope db.RefScope, id string) string {
	return db.MakeRef(family, scope, id)
}
