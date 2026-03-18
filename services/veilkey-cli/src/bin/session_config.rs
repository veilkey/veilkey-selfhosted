use serde::Deserialize;
use std::collections::HashMap;
use std::process;

// ── Config structs ────────────────────────────────────────────────────────────

#[derive(Debug, Deserialize, Default)]
struct Config {
    #[serde(default)]
    proxy: ProxyConfig,
    #[serde(default)]
    tools: HashMap<String, Tool>,
    #[serde(default)]
    session: ProfileDefaults,
    #[serde(default)]
    root_ai: ProfileDefaults,
    #[serde(default)]
    veilkey: VeilkeyConfig,
    #[serde(default)]
    rewrite: RewriteConfig,
}

#[derive(Debug, Deserialize, Default)]
struct ProxyConfig {
    #[serde(default)]
    default: ProxyTarget,
    #[serde(default)]
    tools: HashMap<String, ProxyTarget>,
}

#[derive(Debug, Deserialize, Default, Clone)]
struct ProxyTarget {
    allow_hosts_enabled: Option<bool>,
    #[serde(default)]
    url: String,
    #[serde(default)]
    listen: String,
    #[serde(default)]
    no_proxy: String,
    #[serde(default)]
    allow_hosts: Vec<String>,
    #[serde(default)]
    plaintext_action: String,
    #[serde(default)]
    plaintext_resolve_hosts: Vec<String>,
}

#[derive(Debug, Deserialize, Default)]
struct Tool {
    #[serde(default)]
    bin: String,
    #[serde(default)]
    proxy: String,
}

#[derive(Debug, Deserialize, Default)]
struct ProfileDefaults {
    #[serde(default)]
    default_profile: String,
    #[serde(default)]
    unit_prefix: String,
}

#[derive(Debug, Deserialize, Default)]
struct VeilkeyConfig {
    #[serde(default)]
    localvault_url: String,
    #[serde(default)]
    vaultcenter_url: String,
}

#[derive(Debug, Deserialize, Default)]
struct RewriteConfig {
    #[serde(default)]
    plaintext_action: String,
    #[serde(default)]
    plaintext_resolve_hosts: Vec<String>,
}

// ── Helpers ───────────────────────────────────────────────────────────────────

fn getenv_first(values: &[&str]) -> String {
    for v in values {
        let v = v.trim();
        if !v.is_empty() {
            return v.to_string();
        }
    }
    String::new()
}

fn hostname(raw: &str) -> String {
    if raw.is_empty() {
        return String::new();
    }
    // strip scheme
    let s = if let Some(pos) = raw.find("//") {
        &raw[pos + 2..]
    } else {
        raw
    };
    // strip path / query
    let s = s.split('/').next().unwrap_or(s);
    // strip port
    s.split(':').next().unwrap_or(s).to_string()
}

fn shell_quote(v: &str) -> String {
    format!("'{}'", v.replace('\'', r#"'"'"'"#))
}

fn print_exports(values: &[(&str, &str)]) {
    for (k, v) in values {
        if !v.is_empty() {
            println!("export {}={}", k, shell_quote(v));
        }
    }
}

fn require_value(value: &str, field: &str) -> String {
    let v = value.trim();
    if v.is_empty() {
        eprintln!("{} is not configured", field);
        process::exit(1);
    }
    v.to_string()
}

fn choose_profile_value(primary: &str, secondary: &str, field: &str) -> String {
    let v = if !primary.is_empty() { primary } else { secondary };
    if v.is_empty() {
        eprintln!("{} is not configured", field);
        process::exit(1);
    }
    v.to_string()
}

// ── Config methods ────────────────────────────────────────────────────────────

impl Config {
    fn proxy_url(&self, configured: &str) -> String {
        getenv_first(&[
            &std::env::var("VEILKEY_PROXY_URL").unwrap_or_default(),
            configured,
        ])
    }

    fn proxy_listen(&self, configured: &str) -> String {
        getenv_first(&[
            &std::env::var("VEILKEY_PROXY_LISTEN").unwrap_or_default(),
            configured,
        ])
    }

    fn veilkey_localvault_url(&self) -> String {
        getenv_first(&[
            &std::env::var("VEILKEY_LOCALVAULT_URL").unwrap_or_default(),
            &std::env::var("VEILKEY_API").unwrap_or_default(),
            &self.veilkey.localvault_url,
        ])
    }

    fn veilkey_vaultcenter_url(&self) -> String {
        getenv_first(&[
            &std::env::var("VEILKEY_VAULTCENTER_URL").unwrap_or_default(),
            &self.veilkey.vaultcenter_url,
        ])
    }

    fn tool_proxy(&self, name: &str) -> Result<ProxyTarget, String> {
        let tool_cfg = self
            .tools
            .get(name)
            .ok_or_else(|| format!("unknown tool: {}", name))?;

        let proxy_name = &tool_cfg.proxy;
        if proxy_name.is_empty() || proxy_name == "default" {
            let mut target = self.proxy.default.clone();
            target.url = self.proxy_url(&target.url);
            return Ok(target);
        }
        let cfg = self
            .proxy
            .tools
            .get(proxy_name.as_str())
            .ok_or_else(|| format!("unknown proxy mapping: {}", proxy_name))?
            .clone();
        let mut cfg = cfg;
        cfg.url = self.proxy_url(&cfg.url);
        Ok(cfg)
    }

    fn merged_no_proxy(&self, base: &str) -> String {
        let mut seen = std::collections::HashSet::new();
        let mut order: Vec<String> = Vec::new();

        let mut add = |v: &str| {
            let v = v.trim();
            if !v.is_empty() && seen.insert(v.to_string()) {
                order.push(v.to_string());
            }
        };

        for item in base.split(',') {
            add(item);
        }
        add(&hostname(&self.veilkey_localvault_url()));
        add(&hostname(&self.veilkey_vaultcenter_url()));
        order.join(",")
    }
}

// ── main ─────────────────────────────────────────────────────────────────────

fn main() {
    let default_config = std::env::var("VEILKEY_SESSION_TOOLS_TOML").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_SESSION_TOOLS_TOML is required");
        std::process::exit(1);
    });

    let mut config_path = default_config.clone();
    let raw_args: Vec<String> = std::env::args().collect();

    // Parse --config flag
    let mut remaining: Vec<String> = Vec::new();
    let mut i = 1;
    while i < raw_args.len() {
        if raw_args[i] == "--config" && i + 1 < raw_args.len() {
            config_path = raw_args[i + 1].clone();
            i += 2;
        } else if let Some(v) = raw_args[i].strip_prefix("--config=") {
            config_path = v.to_string();
            i += 1;
        } else {
            remaining.push(raw_args[i].clone());
            i += 1;
        }
    }

    if remaining.is_empty() {
        eprintln!("unknown command: ");
        process::exit(1);
    }

    let data = match std::fs::read_to_string(&config_path) {
        Ok(d) => d,
        Err(e) => {
            eprintln!("{}", e);
            process::exit(1);
        }
    };

    let cfg: Config = match toml::from_str(&data) {
        Ok(c) => c,
        Err(e) => {
            eprintln!("{}", e);
            process::exit(1);
        }
    };

    let cmd = &remaining[0];
    let cmd_args = &remaining[1..];

    match cmd.as_str() {
        "tool-bin" => {
            if cmd_args.is_empty() {
                eprintln!("tool-bin requires tool");
                process::exit(1);
            }
            let tool = cfg.tools.get(&cmd_args[0]).unwrap_or_else(|| {
                eprintln!("unknown tool: {}", cmd_args[0]);
                process::exit(1);
            });
            println!("{}", tool.bin);
        }
        "tool-proxy-url" => {
            if cmd_args.is_empty() {
                eprintln!("tool-proxy-url requires tool");
                process::exit(1);
            }
            let target = cfg.tool_proxy(&cmd_args[0]).unwrap_or_else(|e| {
                eprintln!("{}", e);
                process::exit(1);
            });
            println!(
                "{}",
                require_value(&target.url, &format!("proxy url for tool {}", cmd_args[0]))
            );
        }
        "proxy-listen" => {
            let key = cmd_args.first().map(String::as_str).unwrap_or("default");
            if key == "default" {
                println!(
                    "{}",
                    require_value(
                        &cfg.proxy_listen(&cfg.proxy.default.listen),
                        "proxy listen for profile default"
                    )
                );
            } else {
                let target = cfg.proxy.tools.get(key).unwrap_or_else(|| {
                    eprintln!("unknown proxy profile: {}", key);
                    process::exit(1);
                });
                println!(
                    "{}",
                    require_value(
                        &cfg.proxy_listen(&target.listen),
                        &format!("proxy listen for profile {}", key)
                    )
                );
            }
        }
        "proxy-no-proxy" => {
            let key = cmd_args.first().map(String::as_str).unwrap_or("default");
            if key == "default" {
                println!("{}", cfg.proxy.default.no_proxy);
            } else {
                let target = cfg.proxy.tools.get(key).unwrap_or_else(|| {
                    eprintln!("unknown proxy profile: {}", key);
                    process::exit(1);
                });
                if !target.no_proxy.is_empty() {
                    println!("{}", target.no_proxy);
                } else {
                    println!("{}", cfg.proxy.default.no_proxy);
                }
            }
        }
        "proxy-allow-hosts" => {
            let key = cmd_args.first().map(String::as_str).unwrap_or("default");
            let values = if key == "default" {
                &cfg.proxy.default.allow_hosts
            } else {
                let target = cfg.proxy.tools.get(key).unwrap_or_else(|| {
                    eprintln!("unknown proxy profile: {}", key);
                    process::exit(1);
                });
                &target.allow_hosts
            };
            for host in values {
                println!("{}", host);
            }
        }
        "proxy-allow-hosts-enabled" => {
            let key = cmd_args.first().map(String::as_str).unwrap_or("default");
            let value = if key == "default" {
                cfg.proxy.default.allow_hosts_enabled.unwrap_or(true)
            } else {
                let target = cfg.proxy.tools.get(key).unwrap_or_else(|| {
                    eprintln!("unknown proxy profile: {}", key);
                    process::exit(1);
                });
                target
                    .allow_hosts_enabled
                    .or(cfg.proxy.default.allow_hosts_enabled)
                    .unwrap_or(true)
            };
            println!("{}", value);
        }
        "proxy-plaintext-action" => {
            let key = cmd_args.first().map(String::as_str).unwrap_or("default");
            let value = if key == "default" {
                if !cfg.proxy.default.plaintext_action.is_empty() {
                    cfg.proxy.default.plaintext_action.clone()
                } else {
                    cfg.rewrite.plaintext_action.clone()
                }
            } else {
                let target = cfg.proxy.tools.get(key).unwrap_or_else(|| {
                    eprintln!("unknown proxy profile: {}", key);
                    process::exit(1);
                });
                if !target.plaintext_action.is_empty() {
                    target.plaintext_action.clone()
                } else {
                    cfg.rewrite.plaintext_action.clone()
                }
            };
            println!("{}", value);
        }
        "proxy-plaintext-resolve-hosts" => {
            let key = cmd_args.first().map(String::as_str).unwrap_or("default");
            let values = if key == "default" {
                if !cfg.proxy.default.plaintext_resolve_hosts.is_empty() {
                    cfg.proxy.default.plaintext_resolve_hosts.clone()
                } else {
                    cfg.rewrite.plaintext_resolve_hosts.clone()
                }
            } else {
                let target = cfg.proxy.tools.get(key).unwrap_or_else(|| {
                    eprintln!("unknown proxy profile: {}", key);
                    process::exit(1);
                });
                if !target.plaintext_resolve_hosts.is_empty() {
                    target.plaintext_resolve_hosts.clone()
                } else {
                    cfg.rewrite.plaintext_resolve_hosts.clone()
                }
            };
            for host in &values {
                println!("{}", host);
            }
        }
        "shell-exports" => {
            let proxy_url = require_value(
                &cfg.proxy_url(&cfg.proxy.default.url),
                "proxy url for profile default",
            );
            let no_proxy = cfg.merged_no_proxy(&cfg.proxy.default.no_proxy);
            let localvault_url = cfg.veilkey_localvault_url();
            let vaultcenter_url = cfg.veilkey_vaultcenter_url();
            print_exports(&[
                ("VEILKEY_PROXY_URL", &proxy_url),
                ("HTTP_PROXY", &proxy_url),
                ("HTTPS_PROXY", &proxy_url),
                ("ALL_PROXY", &proxy_url),
                ("NO_PROXY", &no_proxy),
                ("VEILKEY_LOCALVAULT_URL", &localvault_url),
                ("VEILKEY_VAULTCENTER_URL", &vaultcenter_url),
            ]);
        }
        "tool-shell-exports" => {
            if cmd_args.is_empty() {
                eprintln!("tool-shell-exports requires tool");
                process::exit(1);
            }
            let target = cfg.tool_proxy(&cmd_args[0]).unwrap_or_else(|e| {
                eprintln!("{}", e);
                process::exit(1);
            });
            let proxy_url = require_value(
                &target.url,
                &format!("proxy url for tool {}", cmd_args[0]),
            );
            let no_proxy = if !target.no_proxy.is_empty() {
                target.no_proxy.clone()
            } else {
                cfg.proxy.default.no_proxy.clone()
            };
            let no_proxy = cfg.merged_no_proxy(&no_proxy);
            let localvault_url = cfg.veilkey_localvault_url();
            let vaultcenter_url = cfg.veilkey_vaultcenter_url();
            print_exports(&[
                ("VEILKEY_PROXY_URL", &proxy_url),
                ("HTTP_PROXY", &proxy_url),
                ("HTTPS_PROXY", &proxy_url),
                ("ALL_PROXY", &proxy_url),
                ("NO_PROXY", &no_proxy),
                ("VEILKEY_LOCALVAULT_URL", &localvault_url),
                ("VEILKEY_VAULTCENTER_URL", &vaultcenter_url),
            ]);
        }
        "session-default-profile" => {
            println!(
                "{}",
                choose_profile_value(
                    &cfg.session.default_profile,
                    &cfg.root_ai.default_profile,
                    "session.default_profile"
                )
            );
        }
        "session-unit-prefix" => {
            println!(
                "{}",
                choose_profile_value(
                    &cfg.session.unit_prefix,
                    &cfg.root_ai.unit_prefix,
                    "session.unit_prefix"
                )
            );
        }
        "root-ai-default-profile" => {
            println!(
                "{}",
                choose_profile_value(
                    &cfg.root_ai.default_profile,
                    &cfg.session.default_profile,
                    "root_ai.default_profile"
                )
            );
        }
        "root-ai-unit-prefix" => {
            println!(
                "{}",
                choose_profile_value(
                    &cfg.root_ai.unit_prefix,
                    &cfg.session.unit_prefix,
                    "root_ai.unit_prefix"
                )
            );
        }
        "veilkey-localvault-url" => {
            println!("{}", cfg.veilkey_localvault_url());
        }
        "veilkey-vaultcenter-url" => {
            println!("{}", cfg.veilkey_vaultcenter_url());
        }
        "debug-tools" => {
            let mut names: Vec<&String> = cfg.tools.keys().collect();
            names.sort();
            for name in names {
                let bin = std::path::Path::new(&cfg.tools[name].bin)
                    .to_str()
                    .unwrap_or(&cfg.tools[name].bin)
                    .to_string();
                println!("{}\t{}", name, bin);
            }
        }
        unknown => {
            eprintln!("unknown command: {}", unknown);
            process::exit(1);
        }
    }
}
