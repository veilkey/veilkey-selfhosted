package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type config struct {
	Proxy    proxyConfig     `toml:"proxy"`
	Tools    map[string]tool `toml:"tools"`
	Session  profileDefaults `toml:"session"`
	RootAI   profileDefaults `toml:"root_ai"`
	Veilkey  veilkeyConfig   `toml:"veilkey"`
	Rewrite  rewriteConfig   `toml:"rewrite"`
}

type proxyConfig struct {
	Default proxyTarget            `toml:"default"`
	Tools   map[string]proxyTarget `toml:"tools"`
}

type proxyTarget struct {
	AllowHostsEnabled     *bool    `toml:"allow_hosts_enabled"`
	URL                   string   `toml:"url"`
	Listen                string   `toml:"listen"`
	NoProxy               string   `toml:"no_proxy"`
	AllowHosts            []string `toml:"allow_hosts"`
	PlaintextAction       string   `toml:"plaintext_action"`
	PlaintextResolveHosts []string `toml:"plaintext_resolve_hosts"`
}

type tool struct {
	Bin   string `toml:"bin"`
	Proxy string `toml:"proxy"`
}

type profileDefaults struct {
	DefaultProfile string `toml:"default_profile"`
	UnitPrefix     string `toml:"unit_prefix"`
}

type veilkeyConfig struct {
	LocalvaultURL  string `toml:"localvault_url"`
	VaultcenterURL string `toml:"vaultcenter_url"`
}

type rewriteConfig struct {
	PlaintextAction       string   `toml:"plaintext_action"`
	PlaintextResolveHosts []string `toml:"plaintext_resolve_hosts"`
}

func loadConfig(path string) (*config, error) {
	var cfg config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.Tools == nil {
		cfg.Tools = map[string]tool{}
	}
	if cfg.Proxy.Tools == nil {
		cfg.Proxy.Tools = map[string]proxyTarget{}
	}
	return &cfg, nil
}

func getenvFirst(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (c *config) proxyURL(configured string) string {
	return getenvFirst(
		os.Getenv("VEILKEY_PROXY_URL"),
		configured,
	)
}

func (c *config) proxyListen(configured string) string {
	return getenvFirst(
		os.Getenv("VEILKEY_PROXY_LISTEN"),
		configured,
	)
}

func (c *config) veilkeyLocalvaultURL() string {
	return getenvFirst(
		os.Getenv("VEILKEY_LOCALVAULT_URL"),
		os.Getenv("VEILKEY_API"),
		c.Veilkey.LocalvaultURL,
	)
}

func (c *config) veilkeyVaultcenterURL() string {
	return getenvFirst(
		os.Getenv("VEILKEY_VAULTCENTER_URL"),
		c.Veilkey.VaultcenterURL,
	)
}

func (c *config) toolProxy(name string) (proxyTarget, error) {
	toolCfg, ok := c.Tools[name]
	if !ok {
		return proxyTarget{}, fmt.Errorf("unknown tool: %s", name)
	}
	proxyName := toolCfg.Proxy
	if proxyName == "" || proxyName == "default" {
		target := c.Proxy.Default
		target.URL = c.proxyURL(target.URL)
		return target, nil
	}
	cfg, ok := c.Proxy.Tools[proxyName]
	if !ok {
		return proxyTarget{}, fmt.Errorf("unknown proxy mapping: %s", proxyName)
	}
	cfg.URL = c.proxyURL(cfg.URL)
	return cfg, nil
}

func hostname(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func (c *config) mergedNoProxy(base string) string {
	values := map[string]struct{}{}
	order := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := values[v]; ok {
			return
		}
		values[v] = struct{}{}
		order = append(order, v)
	}

	for _, item := range strings.Split(base, ",") {
		add(item)
	}
	for _, raw := range []string{c.veilkeyLocalvaultURL(), c.veilkeyVaultcenterURL()} {
		add(hostname(raw))
	}
	return strings.Join(order, ",")
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'"
}

func printExports(values [][2]string) {
	for _, item := range values {
		if item[1] == "" {
			continue
		}
		fmt.Printf("export %s=%s\n", item[0], shellQuote(item[1]))
	}
}

func chooseProfileValue(primary, secondary, field string) (string, error) {
	value := primary
	if value == "" {
		value = secondary
	}
	if value == "" {
		return "", fmt.Errorf("%s is not configured", field)
	}
	return value, nil
}

func requireConfiguredValue(value, field string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		fmt.Fprintf(os.Stderr, "%s is not configured\n", field)
		os.Exit(1)
	}
	return value
}

func main() {
	defaultConfig := os.Getenv("VEILKEY_SESSION_TOOLS_TOML")
	if defaultConfig == "" {
		defaultConfig = "/etc/veilkey/session-tools.toml"
	}
	configPath := flag.String("config", defaultConfig, "session tools TOML")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "unknown command: ")
		os.Exit(1)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "tool-bin":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "tool-bin requires tool")
			os.Exit(1)
		}
		toolCfg, ok := cfg.Tools[cmdArgs[0]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown tool: %s\n", cmdArgs[0])
			os.Exit(1)
		}
		fmt.Println(toolCfg.Bin)
	case "tool-proxy-url":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "tool-proxy-url requires tool")
			os.Exit(1)
		}
		target, err := cfg.toolProxy(cmdArgs[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(requireConfiguredValue(target.URL, fmt.Sprintf("proxy url for tool %s", cmdArgs[0])))
	case "proxy-listen":
		key := "default"
		if len(cmdArgs) > 0 {
			key = cmdArgs[0]
		}
		if key == "default" {
			fmt.Println(requireConfiguredValue(cfg.proxyListen(cfg.Proxy.Default.Listen), "proxy listen for profile default"))
			return
		}
		target, ok := cfg.Proxy.Tools[key]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown proxy profile: %s\n", key)
			os.Exit(1)
		}
		fmt.Println(requireConfiguredValue(cfg.proxyListen(target.Listen), fmt.Sprintf("proxy listen for profile %s", key)))
	case "proxy-no-proxy":
		key := "default"
		if len(cmdArgs) > 0 {
			key = cmdArgs[0]
		}
		if key == "default" {
			fmt.Println(cfg.Proxy.Default.NoProxy)
			return
		}
		target, ok := cfg.Proxy.Tools[key]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown proxy profile: %s\n", key)
			os.Exit(1)
		}
		if target.NoProxy != "" {
			fmt.Println(target.NoProxy)
		} else {
			fmt.Println(cfg.Proxy.Default.NoProxy)
		}
	case "proxy-allow-hosts":
		key := "default"
		if len(cmdArgs) > 0 {
			key = cmdArgs[0]
		}
		values := cfg.Proxy.Default.AllowHosts
		if key != "default" {
			target, ok := cfg.Proxy.Tools[key]
			if !ok {
				fmt.Fprintf(os.Stderr, "unknown proxy profile: %s\n", key)
				os.Exit(1)
			}
			values = target.AllowHosts
		}
		for _, host := range values {
			fmt.Println(host)
		}
	case "proxy-allow-hosts-enabled":
		key := "default"
		if len(cmdArgs) > 0 {
			key = cmdArgs[0]
		}
		value := true
		if cfg.Proxy.Default.AllowHostsEnabled != nil {
			value = *cfg.Proxy.Default.AllowHostsEnabled
		}
		if key != "default" {
			target, ok := cfg.Proxy.Tools[key]
			if !ok {
				fmt.Fprintf(os.Stderr, "unknown proxy profile: %s\n", key)
				os.Exit(1)
			}
			if target.AllowHostsEnabled != nil {
				value = *target.AllowHostsEnabled
			}
		}
		if value {
			fmt.Println("true")
		} else {
			fmt.Println("false")
		}
	case "proxy-plaintext-action":
		key := "default"
		if len(cmdArgs) > 0 {
			key = cmdArgs[0]
		}
		value := strings.TrimSpace(cfg.Rewrite.PlaintextAction)
		if key == "default" {
			if cfg.Proxy.Default.PlaintextAction != "" {
				value = cfg.Proxy.Default.PlaintextAction
			}
		} else if target, ok := cfg.Proxy.Tools[key]; ok {
			if target.PlaintextAction != "" {
				value = target.PlaintextAction
			}
		} else {
			fmt.Fprintf(os.Stderr, "unknown proxy profile: %s\n", key)
			os.Exit(1)
		}
		fmt.Println(value)
	case "proxy-plaintext-resolve-hosts":
		key := "default"
		if len(cmdArgs) > 0 {
			key = cmdArgs[0]
		}
		values := cfg.Rewrite.PlaintextResolveHosts
		if key == "default" {
			if len(cfg.Proxy.Default.PlaintextResolveHosts) > 0 {
				values = cfg.Proxy.Default.PlaintextResolveHosts
			}
		} else if target, ok := cfg.Proxy.Tools[key]; ok {
			if len(target.PlaintextResolveHosts) > 0 {
				values = target.PlaintextResolveHosts
			}
		} else {
			fmt.Fprintf(os.Stderr, "unknown proxy profile: %s\n", key)
			os.Exit(1)
		}
		for _, host := range values {
			fmt.Println(host)
		}
	case "shell-exports":
		proxyURL := requireConfiguredValue(cfg.proxyURL(cfg.Proxy.Default.URL), "proxy url for profile default")
		printExports([][2]string{
			{"VEILKEY_PROXY_URL", proxyURL},
			{"HTTP_PROXY", proxyURL},
			{"HTTPS_PROXY", proxyURL},
			{"ALL_PROXY", proxyURL},
			{"NO_PROXY", cfg.mergedNoProxy(cfg.Proxy.Default.NoProxy)},
			{"VEILKEY_LOCALVAULT_URL", cfg.veilkeyLocalvaultURL()},
			{"VEILKEY_VAULTCENTER_URL", cfg.veilkeyVaultcenterURL()},
		})
	case "tool-shell-exports":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "tool-shell-exports requires tool")
			os.Exit(1)
		}
		target, err := cfg.toolProxy(cmdArgs[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		proxyURL := requireConfiguredValue(target.URL, fmt.Sprintf("proxy url for tool %s", cmdArgs[0]))
		noProxy := target.NoProxy
		if noProxy == "" {
			noProxy = cfg.Proxy.Default.NoProxy
		}
		printExports([][2]string{
			{"VEILKEY_PROXY_URL", proxyURL},
			{"HTTP_PROXY", proxyURL},
			{"HTTPS_PROXY", proxyURL},
			{"ALL_PROXY", proxyURL},
			{"NO_PROXY", cfg.mergedNoProxy(noProxy)},
			{"VEILKEY_LOCALVAULT_URL", cfg.veilkeyLocalvaultURL()},
			{"VEILKEY_VAULTCENTER_URL", cfg.veilkeyVaultcenterURL()},
		})
	case "session-default-profile":
		value, err := chooseProfileValue(cfg.Session.DefaultProfile, cfg.RootAI.DefaultProfile, "session.default_profile")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(value)
	case "veilkey-localvault-url":
		fmt.Println(cfg.veilkeyLocalvaultURL())
	case "veilkey-vaultcenter-url":
		fmt.Println(cfg.veilkeyVaultcenterURL())
	case "session-unit-prefix":
		value, err := chooseProfileValue(cfg.Session.UnitPrefix, cfg.RootAI.UnitPrefix, "session.unit_prefix")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(value)
	case "root-ai-default-profile":
		value, err := chooseProfileValue(cfg.Session.DefaultProfile, cfg.RootAI.DefaultProfile, "root_ai.default_profile")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(value)
	case "root-ai-unit-prefix":
		value, err := chooseProfileValue(cfg.Session.UnitPrefix, cfg.RootAI.UnitPrefix, "root_ai.unit_prefix")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(value)
	case "debug-tools":
		names := make([]string, 0, len(cfg.Tools))
		for name := range cfg.Tools {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("%s\t%s\n", name, filepath.Clean(cfg.Tools[name].Bin))
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
