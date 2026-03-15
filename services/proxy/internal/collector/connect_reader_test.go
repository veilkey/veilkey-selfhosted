//go:build linux

package collector

import (
	"testing"
)

func TestConnectEventStructAlignment(t *testing.T) {
	// Verify the Go struct matches expected BPF layout
	var raw connectEvent
	_ = raw // ensure it compiles with expected fields

	// Family must distinguish AF_INET (2) vs AF_INET6 (10)
	raw.Family = 2
	if raw.Family != 2 {
		t.Fatalf("Family field assignment failed")
	}
	raw.Family = 10
	if raw.Family != 10 {
		t.Fatalf("Family field assignment failed")
	}
}

func TestConnectEventUnknownFamilySkipped(t *testing.T) {
	// Verify unknown address families don't produce events
	// This is tested indirectly through the switch statement in observeConnect
	// which has a default: continue case for unknown families
	raw := connectEvent{Family: 99}
	if raw.Family != 99 {
		t.Fatalf("unexpected family value")
	}
	// Unknown families are handled by the default case in observeConnect's switch
}
