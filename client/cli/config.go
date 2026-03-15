package main

import (
	"embed"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

//go:embed patterns.yml
var embeddedPatterns embed.FS

type PatternDef struct {
	Name       string `yaml:"name"`
	Regex      string `yaml:"regex"`
	Confidence int    `yaml:"confidence"`
	Group      int    `yaml:"group"`
}

type EntropyConfig struct {
	MinLength       int     `yaml:"min_length"`
	Threshold       float64 `yaml:"threshold"`
	ConfidenceBoost int     `yaml:"confidence_boost"`
}

type SensitiveContext struct {
	Keywords        []string `yaml:"keywords"`
	ConfidenceBoost int      `yaml:"confidence_boost"`
}

type Config struct {
	Patterns         []PatternDef     `yaml:"patterns"`
	Entropy          EntropyConfig    `yaml:"entropy"`
	Excludes         []string         `yaml:"excludes"`
	SensitiveContext SensitiveContext  `yaml:"sensitive_context"`
}

type CompiledPattern struct {
	Name       string
	Regex      *regexp.Regexp
	Confidence int
	Group      int
}

type CompiledConfig struct {
	Patterns         []CompiledPattern
	Entropy          EntropyConfig
	Excludes         []*regexp.Regexp
	SensitiveKeywords []string
	SensitiveBoost   int
}

func LoadConfig(path string) (*CompiledConfig, error) {
	var data []byte
	var err error

	if path != "" {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read patterns file: %w", err)
		}
	} else {
		data, err = embeddedPatterns.ReadFile("patterns.yml")
		if err != nil {
			return nil, fmt.Errorf("cannot read embedded patterns: %w", err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse patterns: %w", err)
	}

	cc := &CompiledConfig{
		Entropy:           cfg.Entropy,
		SensitiveKeywords: make([]string, len(cfg.SensitiveContext.Keywords)),
		SensitiveBoost:    cfg.SensitiveContext.ConfidenceBoost,
	}
	copy(cc.SensitiveKeywords, cfg.SensitiveContext.Keywords)

	if cc.Entropy.MinLength == 0 {
		cc.Entropy.MinLength = 16
	}
	if cc.Entropy.Threshold == 0 {
		cc.Entropy.Threshold = 3.5
	}
	if cc.Entropy.ConfidenceBoost == 0 {
		cc.Entropy.ConfidenceBoost = 20
	}
	if cc.SensitiveBoost == 0 {
		cc.SensitiveBoost = 15
	}

	if cc.Entropy.MinLength < 0 {
		return nil, fmt.Errorf("entropy.min_length must not be negative")
	}
	if cc.Entropy.Threshold < 0 || cc.Entropy.Threshold > 8 {
		return nil, fmt.Errorf("entropy.threshold must be between 0 and 8")
	}
	if cc.Entropy.ConfidenceBoost < 0 || cc.Entropy.ConfidenceBoost > 100 {
		return nil, fmt.Errorf("entropy.confidence_boost must be between 0 and 100")
	}
	if cc.SensitiveBoost < 0 || cc.SensitiveBoost > 100 {
		return nil, fmt.Errorf("sensitive_context.confidence_boost must be between 0 and 100")
	}

	for _, p := range cfg.Patterns {
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: invalid regex for %s: %v\n", p.Name, err)
			continue
		}
		cc.Patterns = append(cc.Patterns, CompiledPattern{
			Name:       p.Name,
			Regex:      re,
			Confidence: p.Confidence,
			Group:      p.Group,
		})
	}

	for _, ex := range cfg.Excludes {
		re, err := regexp.Compile(ex)
		if err != nil {
			continue
		}
		cc.Excludes = append(cc.Excludes, re)
	}

	return cc, nil
}
