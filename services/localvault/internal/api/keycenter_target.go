package api

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type keycenterTarget struct {
	URL      string
	Source   string
	Warnings []string
}

func (s *Server) resolveKeycenterTarget() keycenterTarget {
	candidates := []struct {
		label string
		value string
	}{
		{label: "env:VEILKEY_KEYCENTER_URL", value: strings.TrimSpace(os.Getenv("VEILKEY_KEYCENTER_URL"))},
		{label: "db:VEILKEY_KEYCENTER_URL", value: s.lookupConfigValue("VEILKEY_KEYCENTER_URL")},
	}

	selected := keycenterTarget{}
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
			fmt.Sprintf("keycenter URL drift: using %s from %s, ignored %s from %s",
				selected.URL,
				selected.Source,
				value,
				strings.Join(sources, ", "),
			),
		)
	}
	return selected
}

func (s *Server) logKeycenterTarget(context string) string {
	target := s.resolveKeycenterTarget()
	if target.URL == "" {
		log.Printf("%s: VEILKEY_KEYCENTER_URL not resolved", context)
		return ""
	}
	log.Printf("%s: resolved keycenter=%s source=%s", context, target.URL, target.Source)
	for _, warning := range target.Warnings {
		log.Printf("%s: WARNING %s", context, warning)
	}
	return target.URL
}

func (s *Server) LogResolvedKeycenterURL(context string) string {
	return s.logKeycenterTarget(context)
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
