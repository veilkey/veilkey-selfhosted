package secrets

import "veilkey-localvault/internal/db"

// Package-local aliases for db ref constants — keeps handler code concise.
const (
	refFamilyVK = db.RefFamilyVK

	refScopeLocal = db.RefScopeLocal
	refScopeTemp  = db.RefScopeTemp

	refStatusActive = db.RefStatusActive
	refStatusTemp   = db.RefStatusTemp
	refStatusBlock  = db.RefStatusBlock
)

// makeRef constructs a canonical ref string from its components.
func makeRef(family string, scope db.RefScope, id string) string { return db.MakeRef(family, scope, id) }
