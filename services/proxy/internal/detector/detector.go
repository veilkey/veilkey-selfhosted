package detector

import (
	"regexp"
	"strings"

	"gitlab.ranode.net/veilkey/veilkey-proxy/internal/events"
)

type Rule struct {
	Name    string
	Pattern *regexp.Regexp
}

type Detector struct {
	rules []Rule
}

func New() *Detector {
	return &Detector{
		rules: []Rule{
			{Name: "github_pat", Pattern: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`)},
			{Name: "gitlab_pat", Pattern: regexp.MustCompile(`\bglpat-[A-Za-z0-9\-_]{20,}\b`)},
			{Name: "openai_key", Pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`)},
			{Name: "anthropic_key", Pattern: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9\-_]{20,}\b`)},
			{Name: "aws_access_key", Pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
			{Name: "bearer_literal", Pattern: regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9_\-\.=]{16,}\b`)},
			{Name: "basic_auth_url", Pattern: regexp.MustCompile(`https?://[^/\s:@]+:[^/\s@]+@`)},
		},
	}
}

func (d *Detector) Apply(ev events.Event) events.Event {
	haystack := make([]string, 0, len(ev.Argv)+2)
	haystack = append(haystack, ev.Comm, ev.TargetAddr)
	haystack = append(haystack, ev.Argv...)

	joined := strings.Join(haystack, "\n")
	const maxHaystackLen = 1 << 20 // 1 MiB
	if len(joined) > maxHaystackLen {
		joined = joined[:maxHaystackLen]
	}
	var matches []string
	for _, rule := range d.rules {
		if rule.Pattern.MatchString(joined) {
			matches = append(matches, rule.Name)
		}
	}
	if len(matches) == 0 {
		return ev
	}

	ev.Suspicious = true
	ev.Matches = matches
	return ev
}
