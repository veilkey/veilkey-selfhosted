package mailer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- sanitizeHeader ----------

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no special chars", "Hello World", "Hello World"},
		{"strips CR", "hello\rworld", "helloworld"},
		{"strips LF", "hello\nworld", "helloworld"},
		{"strips both CRLF", "hello\r\nworld", "helloworld"},
		{"multiple injections", "a\rb\nc\r\nd", "abcd"},
		{"empty string", "", ""},
		{"only newlines", "\r\n\r\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeHeader(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeHeader(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- formatMail ----------

func TestFormatMail(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		subject string
		body    string
	}{
		{
			name:    "basic email",
			from:    "sender@example.com",
			to:      "recipient@example.com",
			subject: "Test Subject",
			body:    "Hello, this is a test.",
		},
		{
			name:    "empty body",
			from:    "a@b.com",
			to:      "c@d.com",
			subject: "No body",
			body:    "",
		},
		{
			name:    "multiline body",
			from:    "a@b.com",
			to:      "c@d.com",
			subject: "Multi",
			body:    "line1\r\nline2\r\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMail(tt.from, tt.to, tt.subject, tt.body)

			// Headers must be present
			if !strings.Contains(result, "From: "+sanitizeHeader(tt.from)) {
				t.Errorf("missing From header in: %s", result)
			}
			if !strings.Contains(result, "To: "+sanitizeHeader(tt.to)) {
				t.Errorf("missing To header in: %s", result)
			}
			if !strings.Contains(result, "Subject: "+sanitizeHeader(tt.subject)) {
				t.Errorf("missing Subject header in: %s", result)
			}
			if !strings.Contains(result, "Content-Type: text/plain; charset=utf-8") {
				t.Errorf("missing Content-Type header in: %s", result)
			}

			// Header/body separator
			if !strings.Contains(result, "\r\n\r\n") {
				t.Error("missing header/body separator (blank line)")
			}

			// Body is at the end
			if !strings.HasSuffix(result, tt.body) {
				t.Errorf("result should end with body %q", tt.body)
			}
		})
	}
}

func TestFormatMail_HeaderInjectionPrevented(t *testing.T) {
	// An attacker tries to inject headers via subject with CRLF.
	// sanitizeHeader strips \r and \n, so the injected "Bcc:" becomes
	// part of the Subject value rather than a separate header line.
	result := formatMail("a@b.com", "c@d.com", "Legit\r\nBcc: evil@hack.com", "body")

	// Count how many lines start with a header-like pattern.
	// The injected Bcc should NOT appear on its own line.
	for _, line := range strings.Split(result, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Error("header injection was not prevented: found Bcc: as separate header line")
		}
	}

	// The sanitized subject should be on the Subject: line (no newlines)
	if !strings.Contains(result, "Subject: LegitBcc: evil@hack.com") {
		t.Errorf("sanitized subject not found in result: %s", result)
	}
}

// ---------- readSecretEnv ----------

func TestReadSecretEnv_DirectValue(t *testing.T) {
	t.Setenv("TEST_SECRET_KEY", "direct-value")
	got := readSecretEnv("TEST_SECRET_KEY")
	if got != "direct-value" {
		t.Errorf("got %q, want %q", got, "direct-value")
	}
}

func TestReadSecretEnv_Empty(t *testing.T) {
	// Ensure the env var is not set
	os.Unsetenv("TEST_EMPTY_KEY")
	os.Unsetenv("TEST_EMPTY_KEY_FILE")
	got := readSecretEnv("TEST_EMPTY_KEY")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestReadSecretEnv_FileValue(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("  file-secret-value  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_FILE_KEY_FILE", secretFile)
	os.Unsetenv("TEST_FILE_KEY")

	got := readSecretEnv("TEST_FILE_KEY")
	if got != "file-secret-value" {
		t.Errorf("got %q, want %q", got, "file-secret-value")
	}
}

func TestReadSecretEnv_FileTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_PRIO_KEY", "from-env")
	t.Setenv("TEST_PRIO_KEY_FILE", secretFile)

	got := readSecretEnv("TEST_PRIO_KEY")
	if got != "from-file" {
		t.Errorf("got %q, want %q (file should take precedence)", got, "from-file")
	}
}

func TestReadSecretEnv_MissingFile(t *testing.T) {
	t.Setenv("TEST_MISSING_FILE_KEY_FILE", "/nonexistent/path/to/file")
	os.Unsetenv("TEST_MISSING_FILE_KEY")

	got := readSecretEnv("TEST_MISSING_FILE_KEY")
	if got != "" {
		t.Errorf("got %q, want empty when file doesn't exist", got)
	}
}

func TestReadSecretEnv_WhitespaceValue(t *testing.T) {
	t.Setenv("TEST_WS_KEY", "  trimmed  ")
	got := readSecretEnv("TEST_WS_KEY")
	if got != "trimmed" {
		t.Errorf("got %q, want %q", got, "trimmed")
	}
}

// ---------- Send (config routing) ----------

func TestSend_DefaultFrom(t *testing.T) {
	// When VEILKEY_SMTP_FROM is unset and VEILKEY_SMTP_HOST is unset,
	// Send will try sendmail, which will fail because the binary doesn't exist.
	// But we can verify the error message references the sendmail path.
	os.Unsetenv("VEILKEY_SMTP_HOST")
	os.Unsetenv("VEILKEY_SMTP_FROM")
	t.Setenv("VEILKEY_SENDMAIL", "/nonexistent/sendmail")

	err := Send("test@example.com", "subject", "body")
	if err == nil {
		t.Fatal("expected error when sendmail binary not found")
	}
	if !strings.Contains(err.Error(), "sendmail binary not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSend_SendmailBinaryNotFound(t *testing.T) {
	os.Unsetenv("VEILKEY_SMTP_HOST")
	t.Setenv("VEILKEY_SENDMAIL", "/no/such/binary")

	err := Send("to@example.com", "subject", "body")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sendmail binary not found") {
		t.Errorf("error = %v, want 'sendmail binary not found'", err)
	}
}

func TestSend_CustomSendmailPath(t *testing.T) {
	os.Unsetenv("VEILKEY_SMTP_HOST")
	t.Setenv("VEILKEY_SENDMAIL", "/custom/path/sendmail")

	err := Send("to@example.com", "subject", "body")
	if err == nil {
		t.Fatal("expected error for nonexistent custom sendmail")
	}
	if !strings.Contains(err.Error(), "/custom/path/sendmail") {
		t.Errorf("error should reference custom path, got: %v", err)
	}
}

// ---------- defaults ----------

func TestDefaults(t *testing.T) {
	if defaultSendmailBin != "/usr/sbin/sendmail" {
		t.Errorf("defaultSendmailBin = %q", defaultSendmailBin)
	}
	if defaultSMTPPort != 587 {
		t.Errorf("defaultSMTPPort = %d", defaultSMTPPort)
	}
	if defaultSTARTTLS != true {
		t.Errorf("defaultSTARTTLS = %v", defaultSTARTTLS)
	}
}
