package main

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	PatternsFile string   `yaml:"patterns"`
	ExcludePaths []string `yaml:"exclude_paths"`
	Format       string   `yaml:"format"`
	APIURL       string   `yaml:"api_url"`
	ExitCode     bool     `yaml:"exit_code"`
}

// LoadProjectConfig loads a .veilkey.yml project config.
// Search order: explicit path → .veilkey.yml in CWD → .veilkey.yaml in CWD → ~/.config/veilkey/config.yml.
// Returns (nil, nil) if no config file is found.
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	if path != "" {
		return loadProjectConfigFile(path)
	}

	candidates := []string{
		".veilkey.yml",
		".veilkey.yaml",
	}

	// CWD candidates
	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return loadProjectConfigFile(name)
		}
	}

	// User-level fallback
	home, err := os.UserHomeDir()
	if err == nil {
		userConfig := filepath.Join(home, ".config", "veilkey", "config.yml")
		if _, err := os.Stat(userConfig); err == nil {
			return loadProjectConfigFile(userConfig)
		}
	}

	return nil, nil
}

func loadProjectConfigFile(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
