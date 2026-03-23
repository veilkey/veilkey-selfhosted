package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParsePositiveInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
		err   bool
	}{
		{"0", 0, false},
		{"1", 1, false},
		{"123", 123, false},
		{"500", 500, false},
		{"-1", 0, true},
		{"abc", 0, true},
		{"12.3", 0, true},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, err := parsePositiveInt(tt.input)
		if tt.err && err == nil {
			t.Errorf("parsePositiveInt(%q) = %d, want error", tt.input, got)
		}
		if !tt.err && err != nil {
			t.Errorf("parsePositiveInt(%q) error = %v", tt.input, err)
		}
		if !tt.err && got != tt.want {
			t.Errorf("parsePositiveInt(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestEncodeDecodeStringList(t *testing.T) {
	input := []string{"a", "b", "c"}
	encoded := EncodeStringList(input)
	decoded := DecodeStringList(encoded)

	if len(decoded) != len(input) {
		t.Fatalf("len = %d, want %d", len(decoded), len(input))
	}
	for i, v := range decoded {
		if v != input[i] {
			t.Errorf("decoded[%d] = %q, want %q", i, v, input[i])
		}
	}
}

func TestDecodeStringList_Invalid(t *testing.T) {
	got := DecodeStringList("not json")
	if got != nil {
		t.Errorf("DecodeStringList(invalid) = %v, want nil", got)
	}
}

func TestDecodeStringList_Empty(t *testing.T) {
	got := DecodeStringList("[]")
	if len(got) != 0 {
		t.Errorf("DecodeStringList([]) = %v, want empty", got)
	}
}

func TestRequestBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(r *http.Request)
		want   string
	}{
		{
			"plain http",
			func(r *http.Request) { r.Host = "example.com" },
			"http://example.com",
		},
		{
			"x-forwarded-proto https",
			func(r *http.Request) {
				r.Host = "example.com"
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			"https://example.com",
		},
		{
			"x-forwarded-host",
			func(r *http.Request) {
				r.Host = "internal.local"
				r.Header.Set("X-Forwarded-Host", "public.com")
			},
			"http://public.com",
		},
		{
			"both forwarded headers",
			func(r *http.Request) {
				r.Host = "internal.local"
				r.Header.Set("X-Forwarded-Proto", "https")
				r.Header.Set("X-Forwarded-Host", "public.com")
			},
			"https://public.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			tt.setup(r)
			got := RequestBaseURL(r)
			if got != tt.want {
				t.Errorf("RequestBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseListWindow(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		limit   int
		offset  int
		errMsg  string
	}{
		{"defaults", "", 50, 0, ""},
		{"custom limit", "limit=10", 10, 0, ""},
		{"custom offset", "offset=5", 50, 5, ""},
		{"both", "limit=20&offset=3", 20, 3, ""},
		{"limit too high", "limit=501", 0, 0, "limit must be <= 500"},
		{"invalid limit", "limit=abc", 0, 0, "invalid limit"},
		{"invalid offset", "offset=abc", 0, 0, "invalid offset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+tt.query, nil)
			limit, offset, errMsg := ParseListWindow(r)
			if errMsg != tt.errMsg {
				t.Errorf("errMsg = %q, want %q", errMsg, tt.errMsg)
			}
			if errMsg == "" {
				if limit != tt.limit {
					t.Errorf("limit = %d, want %d", limit, tt.limit)
				}
				if offset != tt.offset {
					t.Errorf("offset = %d, want %d", offset, tt.offset)
				}
			}
		})
	}
}

func TestAgentScheme(t *testing.T) {
	t.Setenv("VEILKEY_AGENT_SCHEME", "")
	t.Setenv("VEILKEY_TLS_CERT", "")
	if got := AgentScheme(); got != "http" {
		t.Errorf("AgentScheme() = %q, want http", got)
	}

	t.Setenv("VEILKEY_TLS_CERT", "/path/to/cert.pem")
	if got := AgentScheme(); got != "https" {
		t.Errorf("AgentScheme() = %q, want https", got)
	}

	t.Setenv("VEILKEY_AGENT_SCHEME", "grpc")
	if got := AgentScheme(); got != "grpc" {
		t.Errorf("AgentScheme() = %q, want grpc (override)", got)
	}
}
