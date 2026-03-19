package api

import (
	"fmt"
	"strings"
	"unicode"

	"veilkey-localvault/internal/db"
)

type RefFamily string

const (
	RefFamilyVK RefFamily = db.RefFamilyVK
	RefFamilyVE RefFamily = db.RefFamilyVE
)

// Package-level aliases for db.RefScope constants — keeps api code concise.
const (
	RefScopeTemp     = db.RefScopeTemp
	RefScopeLocal    = db.RefScopeLocal
	RefScopeExternal = db.RefScopeExternal
)

type ParsedRef struct {
	Raw    string
	Family RefFamily
	Scope  db.RefScope
	ID     string
}

func (r ParsedRef) CanonicalString() string {
	return db.MakeRef(string(r.Family), r.Scope, r.ID)
}

func ParseScopedRef(raw string) (ParsedRef, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return ParsedRef{}, fmt.Errorf("ciphertext is required")
	}

	parts := strings.Split(ref, ":")
	if len(parts) != 3 {
		return ParsedRef{}, fmt.Errorf("ciphertext must use FAMILY:SCOPE:ID")
	}

	parsed := ParsedRef{
		Raw: ref,
		ID:  parts[2],
	}

	switch RefFamily(parts[0]) {
	case RefFamilyVK, RefFamilyVE:
		parsed.Family = RefFamily(parts[0])
	default:
		return ParsedRef{}, fmt.Errorf("family must be VK or VE")
	}

	switch db.RefScope(parts[1]) {
	case RefScopeTemp, RefScopeLocal, RefScopeExternal:
		parsed.Scope = db.RefScope(parts[1])
	default:
		return ParsedRef{}, fmt.Errorf("scope must be TEMP, LOCAL, or EXTERNAL")
	}

	if err := validateRefID(parsed.ID); err != nil {
		return ParsedRef{}, err
	}

	return parsed, nil
}

func ParseScopedVKRef(raw string) (ParsedRef, error) {
	parsed, err := ParseScopedRef(raw)
	if err != nil {
		return ParsedRef{}, err
	}
	if parsed.Family != RefFamilyVK {
		return ParsedRef{}, fmt.Errorf("ciphertext must be a VK ref")
	}
	return parsed, nil
}

func ParseActivationScope(raw string) (db.RefScope, error) {
	switch db.RefScope(strings.TrimSpace(raw)) {
	case RefScopeLocal:
		return RefScopeLocal, nil
	case RefScopeExternal:
		return RefScopeExternal, nil
	default:
		return "", fmt.Errorf("scope must be LOCAL or EXTERNAL")
	}
}

func validateRefID(id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	for _, r := range id {
		switch {
		case unicode.IsDigit(r), unicode.IsLetter(r):
		case r == '.', r == '-', r == '_':
		default:
			return fmt.Errorf("id contains invalid characters")
		}
	}
	return nil
}
