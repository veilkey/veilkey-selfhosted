package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"strings"
	"testing"
)

func TestProcessStreamWithMockAPI(t *testing.T) {
	// Mock VeilKey server
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Return a deterministic VK token in the format Issue() expects
		token := "VK:a1b2c3d4"
		if err := json.NewEncoder(w).Encode(map[string]string{"token": token}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewVeilKeyClient(server.URL)
	logger := NewSessionLogger("/tmp/veilkey-cli-test/filter-session.log")
	detector := NewSecretDetector(cfg, client, logger, false)

	input := strings.NewReader("GITHUB_TOKEN=ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a\nno secret here\n")

	var buf bytes.Buffer
	origStdout := detector.Stats

	// Simulate processStream writing to buffer
	scanner := func() {
		s := newLineScanner(input)
		for s.Scan() {
			line := s.Text()
			processed := detector.ProcessLine(line)
			buf.WriteString(processed)
			buf.WriteByte('\n')
		}
	}
	scanner()

	output := buf.String()

	// GitHub PAT should be replaced
	if strings.Contains(output, "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a") {
		t.Error("GitHub PAT should be replaced in filter output")
	}

	if !strings.Contains(output, "GITHUB_TOKEN=VK:a1b2c3d4") {
		t.Error("assignment key should be preserved while secret value is replaced")
	}

	// VK token should appear
	if !strings.Contains(output, "VK:a1b2c3d4") {
		t.Error("output should contain VK token")
	}

	// Non-secret line should pass through unchanged
	if !strings.Contains(output, "no secret here") {
		t.Error("non-secret line should pass through")
	}

	// Stats should reflect detection
	if detector.Stats.Detections == origStdout.Detections {
		t.Error("detections counter should increase")
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 API issuance for one logical secret, got %d", got)
	}
}

func TestProcessLinePreservesExistingVK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{"token": "VK:ff001122"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	cfg := testConfig()
	client := NewVeilKeyClient(server.URL)
	logger := NewSessionLogger("/tmp/veilkey-cli-test/preserve-session.log")
	detector := NewSecretDetector(cfg, client, logger, false)

	// Line with existing VK token + new secret
	line := "existing=VK:a1b2c3d4 new=ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a"
	processed := detector.ProcessLine(line)

	if !strings.Contains(processed, "VK:a1b2c3d4") {
		t.Error("existing VK token should be preserved")
	}
	if strings.Contains(processed, "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a") {
		t.Error("new secret should be replaced")
	}
}

func TestOutputFormatterText(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("text", &buf)

	f.Header()
	f.FormatFinding(Finding{
		File:       "test.env",
		Line:       5,
		Pattern:    "github-pat",
		Confidence: 90,
		Match:      "ghp_aBcD***",
	})
	f.FormatSummary(Stats{Lines: 100, Detections: 1})
	f.Footer()

	output := buf.String()
	if !strings.Contains(output, "test.env:5") {
		t.Error("should contain file:line")
	}
	if !strings.Contains(output, "github-pat") {
		t.Error("should contain pattern name")
	}
}

func TestOutputFormatterJSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("json", &buf)

	f.Header()
	f.FormatFinding(Finding{
		File:       "test.env",
		Line:       5,
		Pattern:    "github-pat",
		Confidence: 90,
		Match:      "ghp_aBcD***",
	})
	f.FormatSummary(Stats{Lines: 100, Detections: 1})
	f.Footer()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected 2+ JSON lines, got %d", len(lines))
	}

	// Verify first line is valid JSON with type=finding
	var finding map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &finding); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if finding["type"] != "finding" {
		t.Errorf("expected type=finding, got %v", finding["type"])
	}
}

func TestOutputFormatterSARIF(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("sarif", &buf)

	f.Header()
	f.FormatFinding(Finding{
		File:       "test.env",
		Line:       5,
		Pattern:    "github-pat",
		Confidence: 90,
		Match:      "ghp_aBcD***",
	})
	f.FormatSummary(Stats{Lines: 100, Detections: 1})
	f.Footer()

	var sarif map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &sarif); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	if sarif["version"] != "2.1.0" {
		t.Errorf("expected SARIF 2.1.0, got %v", sarif["version"])
	}
}

// newLineScanner is a helper that wraps bufio.Scanner
func newLineScanner(r *strings.Reader) *lineScanner {
	return &lineScanner{r: r}
}

type lineScanner struct {
	r    *strings.Reader
	line string
}

func (s *lineScanner) Scan() bool {
	var buf bytes.Buffer
	for {
		b, err := s.r.ReadByte()
		if err != nil {
			if buf.Len() > 0 {
				s.line = buf.String()
				return true
			}
			return false
		}
		if b == '\n' {
			s.line = buf.String()
			return true
		}
		buf.WriteByte(b)
	}
}

func (s *lineScanner) Text() string {
	return s.line
}
