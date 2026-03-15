package config

import "testing"

func TestValidate(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}

	cfg := Default()
	cfg.Format = "yaml"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid format to fail")
	}
}

func TestValidateCgroup(t *testing.T) {
	cfg := Default()
	cfg.TargetCgroup = "/sys/fs/cgroup/user.slice"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid cgroup should pass: %v", err)
	}

	cfg.TargetCgroup = "/tmp/evil"
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid cgroup prefix should fail")
	}

	cfg.TargetCgroup = "/sys/fs/cgroup/../../etc"
	if err := cfg.Validate(); err == nil {
		t.Fatal("cgroup with .. should fail")
	}
}
