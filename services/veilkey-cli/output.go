package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Finding represents a single detected secret for output formatting.
type Finding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Pattern    string `json:"pattern"`
	Confidence int    `json:"confidence"`
	Match      string `json:"match"` // masked preview
}

// OutputFormatter defines the interface for all output format implementations.
type OutputFormatter interface {
	Header()
	FormatFinding(finding Finding)
	FormatSummary(stats Stats)
	Footer()
}

// NewFormatter creates an OutputFormatter for the given format string.
// Supported formats: "text" (default), "json", "sarif".
// If an unrecognized format is provided, it falls back to text.
func NewFormatter(format string, w io.Writer) OutputFormatter {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return &JSONFormatter{w: w}
	case "sarif":
		return &SARIFFormatter{w: w}
	default:
		return &TextFormatter{w: w}
	}
}

// ---------------------------------------------------------------------------
// TextFormatter — human-readable output to the provided writer
// ---------------------------------------------------------------------------

// TextFormatter writes human-readable scan results.
type TextFormatter struct {
	w     io.Writer
	count int
}

func (f *TextFormatter) Header() {
	fmt.Fprintf(f.w, "\033[0;36m=== veilkey-cli scan ===\033[0m\n\n")
}

func (f *TextFormatter) FormatFinding(finding Finding) {
	f.count++
	loc := finding.File
	if finding.Line > 0 {
		loc = fmt.Sprintf("%s:%d", finding.File, finding.Line)
	}
	fmt.Fprintf(f.w, "\033[0;33m[%d]\033[0m %s\n", f.count, loc)
	fmt.Fprintf(f.w, "    Pattern:    %s\n", finding.Pattern)
	fmt.Fprintf(f.w, "    Confidence: %d\n", finding.Confidence)
	fmt.Fprintf(f.w, "    Match:      %s\n\n", finding.Match)
}

func (f *TextFormatter) FormatSummary(stats Stats) {
	fmt.Fprintf(f.w, "\033[0;36m--- summary ---\033[0m\n")
	fmt.Fprintf(f.w, "Lines scanned: %d\n", stats.Lines)
	fmt.Fprintf(f.w, "Detections:    %d\n", stats.Detections)
	fmt.Fprintf(f.w, "API calls:     %d\n", stats.APICalls)
	if stats.APIErrors > 0 {
		fmt.Fprintf(f.w, "API errors:    \033[0;31m%d\033[0m\n", stats.APIErrors)
	} else {
		fmt.Fprintf(f.w, "API errors:    %d\n", stats.APIErrors)
	}
}

func (f *TextFormatter) Footer() {
	// No footer action needed for text output.
}

// ---------------------------------------------------------------------------
// JSONFormatter — NDJSON output (one JSON object per line)
// ---------------------------------------------------------------------------

// JSONFormatter writes one JSON object per finding (NDJSON / JSON Lines).
type JSONFormatter struct {
	w   io.Writer
	enc *json.Encoder
}

func (f *JSONFormatter) Header() {
	f.enc = json.NewEncoder(f.w)
	f.enc.SetEscapeHTML(false)
}

func (f *JSONFormatter) FormatFinding(finding Finding) {
	if f.enc == nil {
		f.enc = json.NewEncoder(f.w)
		f.enc.SetEscapeHTML(false)
	}
	record := struct {
		Type       string `json:"type"`
		File       string `json:"file"`
		Line       int    `json:"line"`
		Pattern    string `json:"pattern"`
		Confidence int    `json:"confidence"`
		Match      string `json:"match"`
		Timestamp  string `json:"timestamp"`
	}{
		Type:       "finding",
		File:       finding.File,
		Line:       finding.Line,
		Pattern:    finding.Pattern,
		Confidence: finding.Confidence,
		Match:      finding.Match,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	_ = f.enc.Encode(record)
}

func (f *JSONFormatter) FormatSummary(stats Stats) {
	if f.enc == nil {
		f.enc = json.NewEncoder(f.w)
		f.enc.SetEscapeHTML(false)
	}
	record := struct {
		Type       string `json:"type"`
		Lines      int    `json:"lines"`
		Detections int    `json:"detections"`
		APICalls   int    `json:"apiCalls"`
		APIErrors  int    `json:"apiErrors"`
		Timestamp  string `json:"timestamp"`
	}{
		Type:       "summary",
		Lines:      stats.Lines,
		Detections: stats.Detections,
		APICalls:   stats.APICalls,
		APIErrors:  stats.APIErrors,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	_ = f.enc.Encode(record)
}

func (f *JSONFormatter) Footer() {
	// NDJSON has no footer; each line is self-contained.
}

// ---------------------------------------------------------------------------
// SARIFFormatter — OASIS SARIF v2.1.0
// ---------------------------------------------------------------------------

// SARIF data model types (only the subset we need).

type sarifDocument struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string              `json:"name"`
	Version        string              `json:"version"`
	InformationURI string              `json:"informationUri,omitempty"`
	Rules          []sarifRuleMetadata `json:"rules,omitempty"`
}

type sarifRuleMetadata struct {
	ID               string         `json:"id"`
	ShortDescription sarifMessage   `json:"shortDescription"`
	DefaultConfig    sarifRuleLevel `json:"defaultConfiguration"`
}

type sarifRuleLevel struct {
	Level string `json:"level"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// SARIFFormatter buffers all findings and emits a complete SARIF document in Footer().
type SARIFFormatter struct {
	w        io.Writer
	findings []Finding
}

func (f *SARIFFormatter) Header() {
	// Nothing to emit yet; SARIF requires a complete document.
}

func (f *SARIFFormatter) FormatFinding(finding Finding) {
	f.findings = append(f.findings, finding)
}

func (f *SARIFFormatter) FormatSummary(stats Stats) {
	// Summary is not part of the SARIF spec; stats are implicit from results.
	// We could embed them as run.properties, but the spec does not require it.
}

func (f *SARIFFormatter) Footer() {
	// Collect unique rules from findings.
	ruleIndex := make(map[string]int)
	var rules []sarifRuleMetadata
	for _, finding := range f.findings {
		if _, exists := ruleIndex[finding.Pattern]; !exists {
			ruleIndex[finding.Pattern] = len(rules)
			rules = append(rules, sarifRuleMetadata{
				ID:               finding.Pattern,
				ShortDescription: sarifMessage{Text: "Secret pattern: " + finding.Pattern},
				DefaultConfig:    sarifRuleLevel{Level: "error"},
			})
		}
	}

	// Build results.
	results := make([]sarifResult, 0, len(f.findings))
	for _, finding := range f.findings {
		level := confidenceToLevel(finding.Confidence)

		result := sarifResult{
			RuleID:  finding.Pattern,
			Level:   level,
			Message: sarifMessage{Text: fmt.Sprintf("Potential secret detected (confidence: %d): %s", finding.Confidence, finding.Match)},
			Properties: map[string]interface{}{
				"confidence": finding.Confidence,
			},
		}

		if finding.File != "" {
			loc := sarifLocation{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: finding.File},
				},
			}
			if finding.Line > 0 {
				loc.PhysicalLocation.Region = &sarifRegion{StartLine: finding.Line}
			}
			result.Locations = []sarifLocation{loc}
		}

		results = append(results, result)
	}

	doc := sarifDocument{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:    "veilkey-cli",
						Version: version,
						Rules:   rules,
					},
				},
				Results: results,
			},
		},
	}

	enc := json.NewEncoder(f.w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

// confidenceToLevel maps a numeric confidence score to a SARIF severity level.
func confidenceToLevel(confidence int) string {
	switch {
	case confidence >= 80:
		return "error"
	case confidence >= 60:
		return "warning"
	default:
		return "note"
	}
}
