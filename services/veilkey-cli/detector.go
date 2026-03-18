package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

var veilkeyRE = regexp.MustCompile(`VK:(?:(?:TEMP|LOCAL|EXTERNAL):[0-9A-Fa-f]{4,64}|[0-9a-f]{8})`)

type Detection struct {
	Value      string
	FullMatch  string
	Pattern    string
	Confidence int
}

type Stats struct {
	Lines      int
	Detections int
	APICalls   int
	APIErrors  int
}

type WatchEntry struct {
	Value string
	VK    string
}

type SecretDetector struct {
	config    *CompiledConfig
	client    *VeilKeyClient
	logger    *SessionLogger
	scanOnly  bool
	cache     map[string]string
	watchlist []WatchEntry
	Paused    bool // Paused via SIGUSR2 (while vk is running)
	Stats     Stats
}

func NewSecretDetector(config *CompiledConfig, client *VeilKeyClient, logger *SessionLogger, scanOnly bool) *SecretDetector {
	d := &SecretDetector{
		config:   config,
		client:   client,
		logger:   logger,
		scanOnly: scanOnly,
		cache:    make(map[string]string),
	}
	d.loadWatchlist()
	return d
}

// loadWatchlist reads $VEILKEY_STATE_DIR/watchlist (tsv: value\tVK:hash[\texpires_at])
// Entries with an expires_at in the past are skipped; the file is rewritten without them.
func (d *SecretDetector) loadWatchlist() {
	stateDir := os.Getenv("VEILKEY_STATE_DIR")
	if stateDir == "" {
		stateDir = defaultStateDir()
	}
	path := stateDir + "/watchlist"
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	now := time.Now().UTC()
	var kept []string
	pruned := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 || len(parts[0]) < 1 || len(parts[1]) < 1 {
			continue
		}
		// Check expiration if present
		if len(parts) == 3 && parts[2] != "" {
			if exp, err := time.Parse(time.RFC3339, parts[2]); err == nil && now.After(exp) {
				pruned = true
				continue
			}
		}
		d.watchlist = append(d.watchlist, WatchEntry{Value: parts[0], VK: parts[1]})
		kept = append(kept, line)
	}

	if pruned {
		content := strings.Join(kept, "\n")
		if len(kept) > 0 {
			content += "\n"
		}
		os.WriteFile(path, []byte(content), 0644)
	}
}

// ReloadWatchlist re-reads the watchlist file (called on SIGUSR1 or periodically)
func (d *SecretDetector) ReloadWatchlist() {
	d.watchlist = nil
	d.loadWatchlist()
}

func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	counts := make(map[rune]int)
	for _, r := range s {
		counts[r]++
	}
	length := float64(len([]rune(s)))
	var entropy float64
	for _, c := range counts {
		p := float64(c) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func (d *SecretDetector) isExcluded(value string) bool {
	if veilkeyRE.MatchString(value) {
		return true
	}
	for _, ex := range d.config.Excludes {
		if ex.MatchString(value) {
			return true
		}
	}
	return false
}

func (d *SecretDetector) hasSensitiveContext(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range d.config.SensitiveKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (d *SecretDetector) issueVeilKey(value string) string {
	if vk, ok := d.cache[value]; ok {
		return vk
	}

	if d.scanOnly {
		d.cache[value] = "[detected]"
		return "[detected]"
	}

	vk, err := d.client.Issue(value)
	if err != nil {
		d.Stats.APIErrors++
		fmt.Fprintf(errWriter, "WARNING: VeilKey API failed: %v\n", err)
		return ""
	}
	d.Stats.APICalls++
	d.cache[value] = vk
	return vk
}

func (d *SecretDetector) DetectSecrets(line string) []Detection {
	var results []Detection
	hasContext := d.hasSensitiveContext(line)

	for _, pat := range d.config.Patterns {
		matches := pat.Regex.FindAllStringSubmatchIndex(line, -1)
		for _, loc := range matches {
			var value, fullMatch string

			// full match
			fullMatch = line[loc[0]:loc[1]]

			// capture group
			if pat.Group > 0 && pat.Group*2+1 < len(loc) && loc[pat.Group*2] >= 0 {
				value = line[loc[pat.Group*2]:loc[pat.Group*2+1]]
			} else {
				value = fullMatch
			}

			if len(value) < 6 {
				continue
			}
			if d.isExcluded(value) {
				continue
			}

			conf := pat.Confidence
			if hasContext {
				conf += d.config.SensitiveBoost
			}
			if len(value) >= d.config.Entropy.MinLength {
				ent := shannonEntropy(value)
				if ent > d.config.Entropy.Threshold {
					conf += d.config.Entropy.ConfidenceBoost
				}
			}

			if conf >= 40 {
				results = append(results, Detection{
					Value:      value,
					FullMatch:  fullMatch,
					Pattern:    pat.Name,
					Confidence: conf,
				})
			}
		}
	}

	return results
}

func (d *SecretDetector) ProcessLine(line string) string {
	d.Stats.Lines++

	// Protect existing VeilKeys: replace with placeholders
	vkMatches := veilkeyRE.FindAllStringIndex(line, -1)
	var protected []struct {
		placeholder string
		original    string
	}
	if len(vkMatches) > 0 {
		// Replace in reverse order (to preserve indices)
		for i := len(vkMatches) - 1; i >= 0; i-- {
			m := vkMatches[i]
			ph := fmt.Sprintf("\x00VK%d\x00", i)
			orig := line[m[0]:m[1]]
			protected = append(protected, struct {
				placeholder string
				original    string
			}{ph, orig})
			line = line[:m[0]] + ph + line[m[1]:]
		}
	}

	detections := d.DetectSecrets(line)

	if len(detections) > 0 {
		// Sort by confidence desc, then by length desc
		sort.Slice(detections, func(i, j int) bool {
			if detections[i].Confidence != detections[j].Confidence {
				return detections[i].Confidence > detections[j].Confidence
			}
			return len(detections[i].FullMatch) > len(detections[j].FullMatch)
		})

		replaced := make(map[string]bool)
		for _, det := range detections {
			if replaced[det.Value] {
				continue
			}

			vk := d.issueVeilKey(det.Value)
			if vk == "" {
				continue
			}

			if det.Value != det.FullMatch {
				newMatch := strings.Replace(det.FullMatch, det.Value, vk, 1)
				line = strings.Replace(line, det.FullMatch, newMatch, 1)
			} else {
				line = strings.Replace(line, det.Value, vk, 1)
			}

			replaced[det.Value] = true
			d.Stats.Detections++

			preview := det.Value
			if len(preview) > 4 {
				preview = preview[:4] + "***"
			} else {
				preview = "***"
			}
			d.logger.Log(vk, det.Pattern, det.Confidence, preview)
		}
	}

	// Watchlist matching: replace registered values in output (skip when Paused)
	if d.Paused {
		// Restore VeilKey placeholders and return immediately
		for _, p := range protected {
			line = strings.Replace(line, p.placeholder, p.original, 1)
		}
		return line
	}
	for _, w := range d.watchlist {
		if strings.Contains(line, w.Value) {
			line = strings.ReplaceAll(line, w.Value, w.VK)
			d.Stats.Detections++
			preview := w.Value
			if len(preview) > 4 {
				preview = preview[:4] + "***"
			} else {
				preview = "***"
			}
			d.logger.Log(w.VK, "watchlist", 100, preview)
		}
	}

	// Restore VeilKey placeholders
	for _, p := range protected {
		line = strings.Replace(line, p.placeholder, p.original, 1)
	}

	return line
}
