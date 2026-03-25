package db

import "github.com/veilkey/veilkey-go-package/refs"

// Re-export refs package types and constants for backward compatibility.
type RefScope = refs.RefScope
type RefStatus = refs.RefStatus

const RefSep = refs.RefSep

const (
	RefFamilyVK = refs.RefFamilyVK
	RefFamilyVE = refs.RefFamilyVE
)

const (
	RefScopeLocal    = refs.RefScopeLocal
	RefScopeTemp     = refs.RefScopeTemp
	RefScopeExternal = refs.RefScopeExternal
	RefScopeSSH      = RefScope("SSH")
)

const (
	RefStatusActive  = refs.RefStatusActive
	RefStatusTemp    = refs.RefStatusTemp
	RefStatusArchive = refs.RefStatusArchive
	RefStatusBlock   = refs.RefStatusBlock
	RefStatusRevoke  = refs.RefStatusRevoke
)

func MakeRef(family string, scope RefScope, id string) string {
	return refs.MakeRef(family, scope, id)
}

func NormalizeScopeStatus(family string, scope RefScope, status RefStatus, fallbackScope RefScope) (RefScope, RefStatus, error) {
	// SSH scope: treat like LOCAL (active, no expiry)
	if scope == RefScopeSSH {
		if status == "" {
			status = RefStatusActive
		}
		return scope, status, nil
	}
	return refs.NormalizeScopeStatus(family, scope, status, fallbackScope)
}
