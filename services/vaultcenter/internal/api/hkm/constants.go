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

	refStatusActive = db.RefStatusActive
	refStatusTemp   = db.RefStatusTemp
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
func makeRef(family, scope, id string) string { return db.MakeRef(family, scope, id) }
