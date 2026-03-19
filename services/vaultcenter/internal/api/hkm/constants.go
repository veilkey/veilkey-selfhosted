package hkm

import (
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

// Package-local aliases for db ref constants — keeps handler code concise.
const (
	refFamilyVK = db.RefFamilyVK
	refFamilyVE = db.RefFamilyVE

	refScopeLocal    = db.RefScopeLocal
	refScopeTemp     = db.RefScopeTemp
	refScopeExternal = db.RefScopeExternal

	refStatusActive  = db.RefStatusActive
	refStatusTemp    = db.RefStatusTemp
	refStatusArchive = db.RefStatusArchive
	refStatusBlock   = db.RefStatusBlock
	refStatusRevoke  = db.RefStatusRevoke
)

// Package-local aliases for agent API path constants — keeps handler code concise.
const (
	agentPathConfigs      = httputil.AgentPathConfigs
	agentPathConfigsBulk  = httputil.AgentPathConfigsBulk
	agentPathSecrets      = httputil.AgentPathSecrets
	agentPathSecretFields = httputil.AgentPathSecretFields
	agentPathCipher       = httputil.AgentPathCipher
	agentPathResolve      = httputil.AgentPathResolve
	agentPathRekey        = httputil.AgentPathRekey
)

// makeRef constructs a canonical ref string from its components.
func makeRef(family string, scope db.RefScope, id string) string { return db.MakeRef(family, scope, id) }

// refScope converts a string to db.RefScope.
func refScope(s string) db.RefScope { return db.RefScope(s) }

// refStatus converts a string to db.RefStatus.
func refStatus(s string) db.RefStatus { return db.RefStatus(s) }
