package api

import "testing"

func TestResolveKeycenterTargetPrefersEnvOverDB(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("VEILKEY_KEYCENTER_URL", "http://db.example:10180"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	t.Setenv("VEILKEY_KEYCENTER_URL", "http://env.example:10180")

	target := server.resolveKeycenterTarget()
	if target.URL != "http://env.example:10180" {
		t.Fatalf("target.URL = %q", target.URL)
	}
	if target.Source != "env:VEILKEY_KEYCENTER_URL" {
		t.Fatalf("target.Source = %q", target.Source)
	}
	if len(target.Warnings) == 0 {
		t.Fatal("expected drift warnings")
	}
}

func TestResolveKeycenterTargetFallsBackToDB(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("VEILKEY_KEYCENTER_URL", "http://db.example:10180"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	target := server.resolveKeycenterTarget()
	if target.URL != "http://db.example:10180" {
		t.Fatalf("target.URL = %q", target.URL)
	}
	if target.Source != "db:VEILKEY_KEYCENTER_URL" {
		t.Fatalf("target.Source = %q", target.Source)
	}
}
