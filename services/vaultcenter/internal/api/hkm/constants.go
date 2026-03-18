package hkm

import "veilkey-vaultcenter/internal/db"

// Package-local aliases for db ref constants — keeps handler code concise.
const (
	refFamilyVK = db.RefFamilyVK
	refFamilyVE = db.RefFamilyVE

	refScopeLocal    = db.RefScopeLocal
	refScopeTemp     = db.RefScopeTemp
	refScopeExternal = db.RefScopeExternal

	refStatusActive = db.RefStatusActive
	refStatusTemp   = db.RefStatusTemp
)
