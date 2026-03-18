package db

import "fmt"

// RefSep is the canonical separator between family, scope, and ID components.
const RefSep = ":"

// RefScope is a typed ref scope identifier. Using a named type prevents
// arbitrary strings from being assigned to scope fields.
type RefScope string

// RefStatus is a typed ref status value.
type RefStatus string

// Ref family identifiers (untyped — used as plain strings in MakeRef and SQL).
const (
	RefFamilyVK = "VK" // secret refs
	RefFamilyVE = "VE" // config/env refs
)

// Ref scope identifiers.
const (
	RefScopeLocal    RefScope = "LOCAL"
	RefScopeTemp     RefScope = "TEMP"
	RefScopeExternal RefScope = "EXTERNAL"
)

// Ref status values.
const (
	RefStatusActive  RefStatus = "active"
	RefStatusTemp    RefStatus = "temp"
	RefStatusArchive RefStatus = "archive"
	RefStatusBlock   RefStatus = "block"
	RefStatusRevoke  RefStatus = "revoke"
)

// Scan implements sql.Scanner so db/sql can scan string columns directly into RefScope.
func (s *RefScope) Scan(src any) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("RefScope: expected string, got %T", src)
	}
	*s = RefScope(str)
	return nil
}

// Scan implements sql.Scanner so db/sql can scan string columns directly into RefStatus.
func (s *RefStatus) Scan(src any) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("RefStatus: expected string, got %T", src)
	}
	*s = RefStatus(str)
	return nil
}

// MakeRef constructs a canonical ref string: "FAMILY:SCOPE:ID".
func MakeRef(family string, scope RefScope, id string) string {
	return family + RefSep + string(scope) + RefSep + id
}
