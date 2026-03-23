package db

import "testing"

func TestCoalesceString(t *testing.T) {
	tests := []struct {
		value, fallback, want string
	}{
		{"hello", "default", "hello"},
		{"", "default", "default"},
		{"", "", ""},
		{"value", "", "value"},
	}

	for _, tt := range tests {
		got := coalesceString(tt.value, tt.fallback)
		if got != tt.want {
			t.Errorf("coalesceString(%q, %q) = %q, want %q", tt.value, tt.fallback, got, tt.want)
		}
	}
}
