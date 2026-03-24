package plugin

import (
	"testing"
)

func TestDomainRegistry_RegisterAndCheck(t *testing.T) {
	r := NewDomainRegistry()

	// Unregistered domain → ok
	if conflict, ok := r.Check("example.com", "vault-a"); !ok {
		t.Fatalf("expected ok for unregistered domain, got conflict=%s", conflict)
	}

	// Register and check same vault → ok
	r.Register("example.com", "vault-a")
	if conflict, ok := r.Check("example.com", "vault-a"); !ok {
		t.Fatalf("expected ok for same vault, got conflict=%s", conflict)
	}

	// Check from different vault → conflict
	conflict, ok := r.Check("example.com", "vault-b")
	if ok {
		t.Fatal("expected conflict for different vault, got ok")
	}
	if conflict != "vault-a" {
		t.Fatalf("expected conflict vault 'vault-a', got %q", conflict)
	}
}

func TestDomainRegistry_Remove(t *testing.T) {
	r := NewDomainRegistry()
	r.Register("example.com", "vault-a")
	r.Remove("example.com")

	if _, ok := r.Check("example.com", "vault-b"); !ok {
		t.Fatal("expected ok after removal")
	}
}

func TestDomainRegistry_RemoveByVault(t *testing.T) {
	r := NewDomainRegistry()
	r.Register("a.com", "vault-a")
	r.Register("b.com", "vault-a")
	r.Register("c.com", "vault-b")

	r.RemoveByVault("vault-a")

	domains := r.Domains()
	if len(domains) != 1 {
		t.Fatalf("expected 1 domain remaining, got %d", len(domains))
	}
	if domains["c.com"] != "vault-b" {
		t.Fatalf("expected c.com→vault-b, got %v", domains)
	}
}

func TestDomainRegistry_Domains(t *testing.T) {
	r := NewDomainRegistry()
	r.Register("x.com", "v1")
	r.Register("y.com", "v2")

	snap := r.Domains()
	if len(snap) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(snap))
	}

	// Mutating snapshot should not affect registry.
	snap["z.com"] = "v3"
	if len(r.Domains()) != 2 {
		t.Fatal("snapshot mutation affected registry")
	}
}

func TestDomainRegistry_Overwrite(t *testing.T) {
	r := NewDomainRegistry()
	r.Register("example.com", "vault-a")
	r.Register("example.com", "vault-b")

	// Now belongs to vault-b
	if conflict, ok := r.Check("example.com", "vault-b"); !ok {
		t.Fatalf("expected ok after overwrite, got conflict=%s", conflict)
	}
	conflict, ok := r.Check("example.com", "vault-a")
	if ok {
		t.Fatal("expected conflict after overwrite")
	}
	if conflict != "vault-b" {
		t.Fatalf("expected conflict vault 'vault-b', got %q", conflict)
	}
}
