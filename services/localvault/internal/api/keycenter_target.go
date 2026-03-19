package api

import (
	"fmt"
	"log"
	"os"
	"strings"

	"veilkey-localvault/internal/db"
)

type vaultcenterTarget struct {
	URL      string
	Source   string
	Warnings []string
}

func (s *Server) resolveVaultcenterTarget() vaultcenterTarget {
	candidates := []struct {
		label string
		value string
	}{
		{label: "env:VEILKEY_VAULTCENTER_URL", value: strings.TrimSpace(os.Getenv("VEILKEY_VAULTCENTER_URL"))},
		{label: "db:VEILKEY_VAULTCENTER_URL", value: s.lookupConfigValue(db.ConfigKeyVaultcenterURL)},
	}

	selected := vaultcenterTarget{}
	seen := map[string][]string{}
	for _, candidate := range candidates {
		candidate.value = normalizeURL(candidate.value)
		if candidate.value == "" {
			continue
		}
		if selected.URL == "" {
			selected.URL = candidate.value
			selected.Source = candidate.label
		}
		seen[candidate.value] = append(seen[candidate.value], candidate.label)
	}
	if selected.URL == "" {
		return selected
	}

	for value, sources := range seen {
		if value == selected.URL {
			continue
		}
		selected.Warnings = append(
			selected.Warnings,
			fmt.Sprintf("vaultcenter URL drift: using %s from %s, ignored %s from %s",
				selected.URL,
				selected.Source,
				value,
				strings.Join(sources, ", "),
			),
		)
	}
	return selected
}

func (s *Server) logVaultcenterTarget(context string) string {
	target := s.resolveVaultcenterTarget()
	if target.URL == "" {
		log.Printf("%s: VEILKEY_VAULTCENTER_URL not resolved", context)
		return ""
	}
	log.Printf("%s: resolved vaultcenter=%s source=%s", context, target.URL, target.Source)
	for _, warning := range target.Warnings {
		log.Printf("%s: WARNING %s", context, warning)
	}
	return target.URL
}

func (s *Server) LogResolvedVaultcenterURL(context string) string {
	return s.logVaultcenterTarget(context)
}

func (s *Server) lookupConfigValue(key string) string {
	if s == nil || s.db == nil {
		return ""
	}
	cfg, err := s.db.GetConfig(key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Value)
}

func normalizeURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}
