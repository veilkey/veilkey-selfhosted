package api

import (
	"testing"

	"veilkey-localvault/internal/db"
)

func TestParseScopedRef_Valid(t *testing.T) {
	tests := []struct {
		input  string
		family RefFamily
		scope  db.RefScope
		id     string
	}{
		{"VK:LOCAL:abc123", RefFamilyVK, RefScopeLocal, "abc123"},
		{"VK:TEMP:tmp-ref", RefFamilyVK, RefScopeTemp, "tmp-ref"},
		{"VK:EXTERNAL:ext.ref_1", RefFamilyVK, RefScopeExternal, "ext.ref_1"},
		{"VE:LOCAL:ve-ref", RefFamilyVE, RefScopeLocal, "ve-ref"},
		{"VE:TEMP:t", RefFamilyVE, RefScopeTemp, "t"},
		{"  VK:LOCAL:trimmed  ", RefFamilyVK, RefScopeLocal, "trimmed"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parsed, err := ParseScopedRef(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Family != tt.family {
				t.Errorf("family = %q, want %q", parsed.Family, tt.family)
			}
			if parsed.Scope != tt.scope {
				t.Errorf("scope = %q, want %q", parsed.Scope, tt.scope)
			}
			if parsed.ID != tt.id {
				t.Errorf("id = %q, want %q", parsed.ID, tt.id)
			}
		})
	}
}

func TestParseScopedRef_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"one part", "VK"},
		{"two parts", "VK:LOCAL"},
		{"four parts", "VK:LOCAL:abc:extra"},
		{"bad family", "XX:LOCAL:abc"},
		{"bad scope", "VK:INVALID:abc"},
		{"empty id", "VK:LOCAL:"},
		{"invalid id chars", "VK:LOCAL:abc@123"},
		{"id with space", "VK:LOCAL:abc 123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScopedRef(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestParseScopedVKRef_RejectsVE(t *testing.T) {
	_, err := ParseScopedVKRef("VE:LOCAL:abc123")
	if err == nil {
		t.Fatal("expected error for VE family, got nil")
	}
}

func TestParseScopedVKRef_AcceptsVK(t *testing.T) {
	parsed, err := ParseScopedVKRef("VK:LOCAL:abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Family != RefFamilyVK {
		t.Errorf("family = %q, want VK", parsed.Family)
	}
}

func TestParseActivationScope(t *testing.T) {
	tests := []struct {
		input string
		want  db.RefScope
		err   bool
	}{
		{"LOCAL", RefScopeLocal, false},
		{"EXTERNAL", RefScopeExternal, false},
		{"  LOCAL  ", RefScopeLocal, false},
		{"TEMP", "", true},
		{"", "", true},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseActivationScope(tt.input)
			if tt.err && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.err && got != tt.want {
				t.Errorf("scope = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRefID(t *testing.T) {
	valid := []string{"abc", "ABC", "123", "a-b", "a.b", "a_b", "mixedCase123"}
	for _, id := range valid {
		if err := validateRefID(id); err != nil {
			t.Errorf("validateRefID(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{"", "a b", "a@b", "a/b", "a:b", "a!b"}
	for _, id := range invalid {
		if err := validateRefID(id); err == nil {
			t.Errorf("validateRefID(%q) = nil, want error", id)
		}
	}
}

func TestCanonicalString(t *testing.T) {
	ref := ParsedRef{
		Family: RefFamilyVK,
		Scope:  RefScopeLocal,
		ID:     "abc123",
	}
	got := ref.CanonicalString()
	want := "VK:LOCAL:abc123"
	if got != want {
		t.Errorf("CanonicalString() = %q, want %q", got, want)
	}
}
