package plugin

import "sync"

// DomainRegistry tracks domain→vault mappings across all vaults,
// enabling duplicate domain detection at the VaultCenter level.
type DomainRegistry struct {
	mu      sync.RWMutex
	domains map[string]string // domain → vault hash
}

// NewDomainRegistry creates an empty domain registry.
func NewDomainRegistry() *DomainRegistry {
	return &DomainRegistry{
		domains: make(map[string]string),
	}
}

// Register associates a domain with a vault hash.
// If the domain was previously registered to a different vault, it is overwritten.
func (r *DomainRegistry) Register(domain, vault string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains[domain] = vault
}

// Check returns whether the domain conflicts with an existing registration.
// If the domain is registered to a different vault, it returns the conflicting
// vault hash and ok=false. If not registered or registered to the same vault,
// it returns ok=true.
func (r *DomainRegistry) Check(domain, vault string) (conflictVault string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	existing, found := r.domains[domain]
	if !found || existing == vault {
		return "", true
	}
	return existing, false
}

// Remove deletes a domain from the registry.
func (r *DomainRegistry) Remove(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.domains, domain)
}

// RemoveByVault removes all domains associated with a vault.
func (r *DomainRegistry) RemoveByVault(vault string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for domain, v := range r.domains {
		if v == vault {
			delete(r.domains, domain)
		}
	}
}

// Domains returns a snapshot of all registered domain→vault mappings.
func (r *DomainRegistry) Domains() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.domains))
	for k, v := range r.domains {
		out[k] = v
	}
	return out
}
