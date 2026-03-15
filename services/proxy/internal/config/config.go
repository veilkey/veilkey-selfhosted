package config

import (
	"fmt"
	"strings"
)

type Config struct {
	TargetUID    uint
	TargetCgroup string
	Format       string
	Once         bool
	OnlySuspicious bool
	EnforceKill    bool
}

func Default() Config {
	return Config{
		Format: "text",
	}
}

func (c Config) Validate() error {
	switch c.Format {
	case "text", "json":
	default:
		return fmt.Errorf("unsupported format: %s", c.Format)
	}
	if c.TargetCgroup != "" {
		cg := strings.TrimSpace(c.TargetCgroup)
		if !strings.HasPrefix(cg, "/sys/fs/cgroup/") {
			return fmt.Errorf("target_cgroup must start with /sys/fs/cgroup/")
		}
		if strings.Contains(cg, "..") {
			return fmt.Errorf("target_cgroup must not contain '..'")
		}
	}
	return nil
}
