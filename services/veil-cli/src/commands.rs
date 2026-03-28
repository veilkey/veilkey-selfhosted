use std::io::{self, BufRead, Read};
use std::process;

use crate::api::VeilKeyClient;
use crate::config::load_config;
use crate::detector::SecretDetector;
use crate::logger::SessionLogger;
use crate::output::{Finding, Formatter};
use crate::state::{current_paste_mode, set_paste_mode};

const SCAN_PREVIEW_LEN: usize = 8;

fn process_stream(detector: &mut SecretDetector, r: impl Read) {
    let reader = io::BufReader::new(r);
    for line in reader.lines() {
        match line {
            Ok(l) => println!("{}", detector.process_line(&l)),
            Err(_) => break,
        }
    }
}

pub fn cmd_wrap(args: &[String], api_url: &str, log_path: &str, patterns_file: Option<&str>) {
    if args.is_empty() {
        eprintln!("Usage: veilkey wrap <command...>");
        process::exit(1);
    }
    let cfg = match load_config(patterns_file) {
        Ok(c) => c,
        Err(e) => {
            eprintln!("ERROR: {}", e);
            process::exit(1);
        }
    };
    let client = VeilKeyClient::new(api_url);
    let logger = SessionLogger::new(log_path);
    let mut detector = SecretDetector::new(&cfg, &client, &logger, false);

    let mut child = match process::Command::new(&args[0])
        .args(&args[1..])
        .stdin(process::Stdio::inherit())
        .stderr(process::Stdio::inherit())
        .stdout(process::Stdio::piped())
        .spawn()
    {
        Ok(c) => c,
        Err(e) => {
            eprintln!("ERROR: {}", e);
            process::exit(1);
        }
    };

    if let Some(stdout) = child.stdout.take() {
        process_stream(&mut detector, stdout);
    }

    let exit_code = match child.wait() {
        Ok(status) => status.code().unwrap_or(1),
        Err(_) => 1,
    };

    if detector.stats.detections > 0 {
        eprintln!(
            "\n[veilkey] {} secret(s) detected and replaced",
            detector.stats.detections
        );
    }
    process::exit(exit_code);
}

pub fn cmd_exec(args: &[String], api_url: &str, log_path: &str, patterns_file: Option<&str>) {
    if args.is_empty() {
        eprintln!("Usage: veilkey exec <command...>");
        process::exit(1);
    }
    let client = VeilKeyClient::new(api_url);
    let vk_re = regex::Regex::new(crate::detector::VEILKEY_RE_STR).unwrap();

    let mut mask_pairs: Vec<(String, String)> = Vec::new();

    let resolved: Vec<String> = args
        .iter()
        .map(|arg| {
            vk_re
                .replace_all(arg, |caps: &regex::Captures| {
                    match client.resolve(&caps[0]) {
                        Ok(v) => {
                            mask_pairs.push((v.clone(), caps[0].to_string()));
                            v
                        }
                        Err(e) => {
                            eprintln!("WARNING: resolve {} failed: {}", &caps[0], e);
                            caps[0].to_string()
                        }
                    }
                })
                .to_string()
        })
        .collect();

    // Mask stdout to prevent resolved plaintext from leaking to terminal
    let cfg = match load_config(patterns_file) {
        Ok(c) => c,
        Err(_) => {
            // Fallback: run without masking rather than blocking execution
            let status = process::Command::new(&resolved[0])
                .args(&resolved[1..])
                .stdin(process::Stdio::inherit())
                .stdout(process::Stdio::inherit())
                .stderr(process::Stdio::inherit())
                .status();
            match status {
                Ok(s) => process::exit(s.code().unwrap_or(1)),
                Err(_) => process::exit(1),
            }
        }
    };

    let logger = SessionLogger::new(log_path);
    let mut detector = SecretDetector::new(&cfg, &client, &logger, false);

    for (plaintext, vk_ref) in &mask_pairs {
        detector.register_known(plaintext, vk_ref);
    }

    let cfg2 = load_config(patterns_file).ok();
    let mask_pairs_clone = mask_pairs.clone();

    let mut child = match process::Command::new(&resolved[0])
        .args(&resolved[1..])
        .stdin(process::Stdio::inherit())
        .stdout(process::Stdio::piped())
        .stderr(process::Stdio::piped())
        .spawn()
    {
        Ok(c) => c,
        Err(e) => {
            eprintln!("ERROR: {}", e);
            process::exit(1);
        }
    };

    // Mask stderr in a separate thread
    let stderr_handle = child.stderr.take().map(|stderr| {
        let api = api_url.to_string();
        let lp = log_path.to_string();
        std::thread::spawn(move || {
            if let Some(c2) = cfg2 {
                let cl2 = VeilKeyClient::new(&api);
                let lg2 = SessionLogger::new(&lp);
                let mut det2 = SecretDetector::new(&c2, &cl2, &lg2, false);
                for (p, v) in &mask_pairs_clone {
                    det2.register_known(p, v);
                }
                let reader = io::BufReader::new(stderr);
                for line in reader.lines() {
                    match line {
                        Ok(l) => eprintln!("{}", det2.process_line(&l)),
                        Err(_) => break,
                    }
                }
            } else {
                // Fallback: pass through unmasked
                let reader = io::BufReader::new(stderr);
                for line in reader.lines() {
                    match line {
                        Ok(l) => eprintln!("{}", l),
                        Err(_) => break,
                    }
                }
            }
        })
    });

    if let Some(stdout) = child.stdout.take() {
        process_stream(&mut detector, stdout);
    }

    if let Some(h) = stderr_handle {
        let _ = h.join();
    }

    let exit_code = match child.wait() {
        Ok(status) => status.code().unwrap_or(1),
        Err(_) => 1,
    };
    process::exit(exit_code);
}

pub fn cmd_scan(
    file: &str,
    _api_url: &str,
    log_path: &str,
    patterns_file: Option<&str>,
    output_format: &str,
    exit_code_flag: bool,
) {
    let cfg = match load_config(patterns_file) {
        Ok(c) => c,
        Err(e) => {
            eprintln!("ERROR: {}", e);
            process::exit(1);
        }
    };
    let client = VeilKeyClient::new("");
    let logger = SessionLogger::new(log_path);
    let mut detector = SecretDetector::new(&cfg, &client, &logger, true);
    let mut formatter = Formatter::new(output_format, io::stdout());

    formatter.header();

    if file == "-" {
        scan_reader(
            &mut detector,
            &mut formatter,
            Box::new(io::stdin()),
            "<stdin>",
        );
    } else {
        let path = std::path::Path::new(file);
        if path.is_dir() {
            scan_dir_recursive(&mut detector, &mut formatter, path);
        } else {
            match std::fs::File::open(file) {
                Ok(f) => scan_reader(&mut detector, &mut formatter, Box::new(f), file),
                Err(_) => {
                    eprintln!("ERROR: file not found: {}", file);
                    process::exit(1);
                }
            }
        }
    }

    formatter.format_summary(&detector.stats);
    formatter.footer();

    if exit_code_flag && detector.stats.detections > 0 {
        process::exit(1);
    }
}

fn scan_dir_recursive(
    detector: &mut SecretDetector,
    formatter: &mut Formatter<impl io::Write>,
    dir: &std::path::Path,
) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(e) => {
            eprintln!("WARNING: cannot read directory {}: {}", dir.display(), e);
            return;
        }
    };
    let mut paths: Vec<std::path::PathBuf> =
        entries.filter_map(|e| e.ok().map(|e| e.path())).collect();
    paths.sort();
    for path in paths {
        let name = path.file_name().and_then(|n| n.to_str()).unwrap_or("");
        // Skip hidden dirs, .git, node_modules, target, vendor
        if name.starts_with('.') || name == "node_modules" || name == "target" || name == "vendor" {
            continue;
        }
        if path.is_dir() {
            scan_dir_recursive(detector, formatter, &path);
        } else if path.is_file() {
            // Skip binary-looking files
            if is_likely_binary(&path) {
                continue;
            }
            if let Ok(f) = std::fs::File::open(&path) {
                scan_reader(detector, formatter, Box::new(f), &path.to_string_lossy());
            }
        }
    }
}

fn is_likely_binary(path: &std::path::Path) -> bool {
    let binary_exts = [
        "png", "jpg", "jpeg", "gif", "bmp", "ico", "webp", "svg", "woff", "woff2", "ttf", "eot",
        "otf", "zip", "tar", "gz", "bz2", "xz", "7z", "rar", "exe", "dll", "so", "dylib", "o", "a",
        "pdf", "doc", "docx", "xls", "xlsx", "db", "sqlite", "sqlite3", "wasm", "class", "pyc",
    ];
    path.extension()
        .and_then(|e| e.to_str())
        .map(|e| binary_exts.contains(&e.to_lowercase().as_str()))
        .unwrap_or(false)
}

fn scan_reader(
    detector: &mut SecretDetector,
    formatter: &mut Formatter<impl io::Write>,
    reader: Box<dyn Read>,
    file_name: &str,
) {
    let buf_reader = io::BufReader::new(reader);
    for (line_num, line) in buf_reader.lines().enumerate() {
        let line = match line {
            Ok(l) => l,
            Err(_) => break,
        };
        let line_num = line_num + 1;
        let detections = detector.detect_secrets(&line);
        detector.stats.lines += 1;
        for det in detections {
            detector.stats.detections += 1;
            let preview = if det.value.chars().count() > SCAN_PREVIEW_LEN {
                let end: usize = det
                    .value
                    .char_indices()
                    .nth(SCAN_PREVIEW_LEN)
                    .map(|(i, _)| i)
                    .unwrap_or(det.value.len());
                format!("{}***", &det.value[..end])
            } else {
                det.value.clone()
            };
            formatter.format_finding(Finding {
                file: file_name.to_string(),
                line: line_num,
                pattern: det.pattern.clone(),
                confidence: det.confidence,
                r#match: preview,
            });
        }
    }
}

pub fn cmd_filter(file: &str, api_url: &str, log_path: &str, patterns_file: Option<&str>) {
    let cfg = match load_config(patterns_file) {
        Ok(c) => c,
        Err(e) => {
            eprintln!("ERROR: {}", e);
            process::exit(1);
        }
    };
    let client = VeilKeyClient::new(api_url);
    let logger = SessionLogger::new(log_path);
    let entries_before = logger.count();
    let mut detector = SecretDetector::new(&cfg, &client, &logger, false);

    let reader: Box<dyn Read> = if file == "-" {
        Box::new(io::stdin())
    } else {
        match std::fs::File::open(file) {
            Ok(f) => Box::new(f),
            Err(_) => {
                eprintln!("ERROR: file not found: {}", file);
                process::exit(1);
            }
        }
    };

    process_stream(&mut detector, reader);

    if detector.stats.detections > 0 {
        let all_entries = logger.read_entries();
        let new_entries = if entries_before < all_entries.len() {
            &all_entries[entries_before..]
        } else {
            &[]
        };
        let temp_count = new_entries
            .iter()
            .filter(|e| e.veilkey.contains(":TEMP:"))
            .count();

        if temp_count > 0 {
            eprintln!(
                "\n\x1b[1;33m[veilkey] WARNING: {} of {} secret(s) replaced with VK:TEMP references.\x1b[0m",
                temp_count, detector.stats.detections
            );
            eprintln!(
                "\x1b[33m  TEMP refs expire and should NOT be written to config files.\x1b[0m"
            );
            eprintln!(
                "\x1b[33m  Use 'POST /api/activate' to convert TEMP → LOCAL before saving.\x1b[0m"
            );
        } else {
            eprintln!(
                "\n[veilkey] {} secret(s) detected and replaced",
                detector.stats.detections
            );
        }
    }
}

pub fn cmd_list(log_path: &str) {
    let logger = SessionLogger::new(log_path);
    let entries = logger.read_entries();
    if entries.is_empty() {
        println!("No secrets detected");
        return;
    }
    println!(
        "\x1b[0;36m{:<20} {:<25} {:<8} TIMESTAMP\x1b[0m",
        "VEILKEY", "PATTERN", "CONF"
    );
    println!("{}", "─".repeat(70));
    for e in &entries {
        println!(
            "\x1b[0;32m{:<20}\x1b[0m {:<25} {:<8} {}",
            e.veilkey, e.pattern, e.confidence, e.timestamp
        );
    }
    println!("\nTotal: {} VeilKey(s)", entries.len());
}

pub fn cmd_clear(log_path: &str) {
    let logger = SessionLogger::new(log_path);
    let _ = logger.clear();
    println!("Session log cleared");
}

pub fn cmd_paste_mode(args: &[String]) {
    if args.is_empty() || args[0] == "status" {
        println!("paste-mode: {}", current_paste_mode());
        return;
    }
    if args.len() > 1 {
        eprintln!("Usage: veilkey paste-mode [on|off|status]");
        process::exit(1);
    }
    if let Err(e) = set_paste_mode(&args[0]) {
        eprintln!("ERROR: {}", e);
        process::exit(1);
    }
    println!("paste-mode: {}", current_paste_mode());
}

pub fn cmd_status(api_url: &str, log_path: &str, patterns_file: Option<&str>, version: &str) {
    println!("\x1b[0;36m=== veilkey ===\x1b[0m");
    println!();
    println!("Version: {}", version);
    println!("API:     {}", api_url);
    println!("Log:     {}", log_path);
    println!("Paste:   {}", current_paste_mode());
    println!();
    let logger = SessionLogger::new(log_path);
    println!("Secrets: {} detected", logger.count());
    let client = VeilKeyClient::new(api_url);
    if client.health_check() {
        println!("API:     \x1b[0;32mconnected\x1b[0m");
    } else {
        println!("API:     \x1b[0;31munreachable\x1b[0m");
    }
    if let Ok(cfg) = load_config(patterns_file) {
        println!("Patterns: {} loaded", cfg.patterns.len());
    }
}

pub fn cmd_traefik(args: &[String], api_url: &str, _log_path: &str, _patterns_file: Option<&str>) {
    if args.is_empty() {
        eprintln!("Usage: veilkey traefik <init|status|destroy>");
        eprintln!("  init    --domain <domain> --dns cloudflare    Initialize Traefik config");
        eprintln!("  status                                        Show Traefik route status");
        eprintln!("  destroy [--purge]                             Remove Traefik config");
        process::exit(1);
    }
    match args[0].as_str() {
        "init" => traefik_init(&args[1..], api_url),
        "status" => traefik_status(api_url),
        "destroy" => traefik_destroy(&args[1..], api_url),
        other => {
            eprintln!("Unknown traefik subcommand: {}", other);
            process::exit(1);
        }
    }
}

fn traefik_init(args: &[String], api_url: &str) {
    let mut domain: Option<String> = None;
    let mut dns: Option<String> = None;
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--domain" if i + 1 < args.len() => {
                domain = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--domain=") => {
                domain = Some(a["--domain=".len()..].to_string());
                i += 1;
            }
            "--dns" if i + 1 < args.len() => {
                dns = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--dns=") => {
                dns = Some(a["--dns=".len()..].to_string());
                i += 1;
            }
            _ => {
                i += 1;
            }
        }
    }
    let domain = match domain {
        Some(d) => d,
        None => {
            eprintln!("ERROR: --domain is required");
            eprintln!("Usage: veilkey traefik init --domain <domain> --dns <provider>");
            process::exit(1);
        }
    };
    let dns = dns.unwrap_or_else(|| "cloudflare".to_string());

    let client = VeilKeyClient::new(api_url);
    let url = format!("{}/api/plugins/traefik-sync/api/routes", client.base_url());
    let body = serde_json::json!({ "domain": domain, "dns": dns });
    match client.raw_post(&url, &body) {
        Ok(resp) => {
            let result: serde_json::Value = resp.into_json().unwrap_or_default();
            println!("Traefik config initialized: domain={}, dns={}", domain, dns);
            if let Some(msg) = result["message"].as_str() {
                println!("  {}", msg);
            }
        }
        Err(e) => {
            eprintln!("ERROR: traefik init failed: {}", e);
            process::exit(1);
        }
    }
}

fn traefik_status(api_url: &str) {
    let client = VeilKeyClient::new(api_url);
    let url = format!("{}/api/plugins/traefik-sync/api/routes", client.base_url());
    match client.raw_get(&url) {
        Ok(resp) => {
            let result: serde_json::Value = resp.into_json().unwrap_or_default();
            let routes = result["routes"].as_array();
            match routes {
                Some(routes) if !routes.is_empty() => {
                    println!(
                        "\x1b[0;36m{:<20} {:<30} {:<30} {:<6}\x1b[0m",
                        "NAME", "DOMAIN", "BACKEND", "TLS"
                    );
                    println!("{}", "─".repeat(86));
                    for r in routes {
                        println!(
                            "{:<20} {:<30} {:<30} {:<6}",
                            r["name"].as_str().unwrap_or("-"),
                            r["domain"].as_str().unwrap_or("-"),
                            r["backend"].as_str().unwrap_or("-"),
                            if r["tls"].as_bool().unwrap_or(false) {
                                "yes"
                            } else {
                                "no"
                            },
                        );
                    }
                    println!("\nTotal: {} route(s)", routes.len());
                }
                _ => {
                    println!("No Traefik routes configured");
                }
            }
        }
        Err(e) => {
            eprintln!("ERROR: traefik status failed: {}", e);
            process::exit(1);
        }
    }
}

fn traefik_destroy(args: &[String], api_url: &str) {
    let purge = args.iter().any(|a| a == "--purge");

    let client = VeilKeyClient::new(api_url);
    let mut url = format!("{}/api/plugins/traefik-sync/api/routes", client.base_url());
    if purge {
        url.push_str("?purge=true");
    }
    match client.raw_delete(&url) {
        Ok(resp) => {
            let result: serde_json::Value = resp.into_json().unwrap_or_default();
            if purge {
                println!("Traefik config destroyed (purge)");
            } else {
                println!("Traefik config destroyed");
            }
            if let Some(msg) = result["message"].as_str() {
                println!("  {}", msg);
            }
        }
        Err(e) => {
            eprintln!("ERROR: traefik destroy failed: {}", e);
            process::exit(1);
        }
    }
}

pub fn cmd_plugin(args: &[String], api_url: &str, _log_path: &str, _patterns_file: Option<&str>) {
    if args.is_empty() {
        eprintln!("Usage: veilkey plugin <list|sync> [args]");
        eprintln!("  list");
        eprintln!("  sync --vault <vault> --plugin <name> --action <action> --input-json <json>");
        eprintln!("  sync --vault <vault> --plugin <name> --action <action> --input-file <path>");
        process::exit(1);
    }
    match args[0].as_str() {
        "list" => plugin_list(api_url),
        "sync" => plugin_sync(&args[1..], api_url),
        other => {
            eprintln!("Unknown plugin subcommand: {}", other);
            process::exit(1);
        }
    }
}

fn plugin_list(api_url: &str) {
    let client = VeilKeyClient::new(api_url);
    let password = std::env::var("VEILKEY_PASSWORD")
        .unwrap_or_else(|_| rpassword::prompt_password("VeilKey password: ").unwrap_or_default());
    if let Err(e) = client.admin_login(&password) {
        eprintln!("[veilkey] login failed: {}", e);
        process::exit(1);
    }
    match client.plugins_list() {
        Ok(plugins) => {
            if plugins.is_empty() {
                println!("No plugins installed");
                return;
            }
            println!(
                "\x1b[0;36m{:<24} {:<10} {:<8} {}\x1b[0m",
                "NAME", "VERSION", "LOADED", "DESCRIPTION"
            );
            println!("{}", "─".repeat(80));
            for plugin in plugins {
                println!(
                    "{:<24} {:<10} {:<8} {}",
                    plugin["name"].as_str().unwrap_or("-"),
                    plugin["version"].as_str().unwrap_or("-"),
                    if plugin["loaded"].as_bool().unwrap_or(false) {
                        "yes"
                    } else {
                        "no"
                    },
                    plugin["description"].as_str().unwrap_or("-"),
                );
            }
        }
        Err(e) => {
            eprintln!("ERROR: plugin list failed: {}", e);
            process::exit(1);
        }
    }
}

fn plugin_sync(args: &[String], api_url: &str) {
    let mut vault: Option<String> = None;
    let mut plugin: Option<String> = None;
    let mut action: Option<String> = None;
    let mut input_json: Option<String> = None;
    let mut input_file: Option<String> = None;
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--vault" if i + 1 < args.len() => {
                vault = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--vault=") => {
                vault = Some(a["--vault=".len()..].to_string());
                i += 1;
            }
            "--plugin" if i + 1 < args.len() => {
                plugin = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--plugin=") => {
                plugin = Some(a["--plugin=".len()..].to_string());
                i += 1;
            }
            "--action" if i + 1 < args.len() => {
                action = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--action=") => {
                action = Some(a["--action=".len()..].to_string());
                i += 1;
            }
            "--input-json" if i + 1 < args.len() => {
                input_json = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--input-json=") => {
                input_json = Some(a["--input-json=".len()..].to_string());
                i += 1;
            }
            "--input-file" if i + 1 < args.len() => {
                input_file = Some(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--input-file=") => {
                input_file = Some(a["--input-file=".len()..].to_string());
                i += 1;
            }
            _ => {
                i += 1;
            }
        }
    }

    let vault = vault.unwrap_or_else(|| {
        eprintln!("ERROR: --vault is required");
        process::exit(1);
    });
    let plugin = plugin.unwrap_or_else(|| {
        eprintln!("ERROR: --plugin is required");
        process::exit(1);
    });
    let action = action.unwrap_or_else(|| {
        eprintln!("ERROR: --action is required");
        process::exit(1);
    });

    let raw_input = if let Some(path) = input_file {
        std::fs::read_to_string(&path).unwrap_or_else(|e| {
            eprintln!("ERROR: cannot read {}: {}", path, e);
            process::exit(1);
        })
    } else if let Some(json) = input_json {
        json
    } else {
        eprintln!("ERROR: --input-json or --input-file is required");
        process::exit(1);
    };

    let input: serde_json::Value = serde_json::from_str(&raw_input).unwrap_or_else(|e| {
        eprintln!("ERROR: invalid input JSON: {}", e);
        process::exit(1);
    });

    let client = VeilKeyClient::new(api_url);
    let password = std::env::var("VEILKEY_PASSWORD")
        .unwrap_or_else(|_| rpassword::prompt_password("VeilKey password: ").unwrap_or_default());
    if let Err(e) = client.admin_login(&password) {
        eprintln!("[veilkey] login failed: {}", e);
        process::exit(1);
    }
    match client.plugin_sync(&vault, &plugin, &action, &input) {
        Ok(result) => {
            println!(
                "plugin synced: vault={} plugin={} action={}",
                vault, plugin, action
            );
            println!(
                "{}",
                serde_json::to_string_pretty(&result).unwrap_or_else(|_| result.to_string())
            );
        }
        Err(e) => {
            eprintln!("ERROR: plugin sync failed: {}", e);
            process::exit(1);
        }
    }
}

pub fn cmd_proxy(args: &[String]) {
    let mut listen = String::new();
    let mut allow_hosts: Vec<String> = Vec::new();
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--listen" if i + 1 < args.len() => {
                listen = args[i + 1].clone();
                i += 2;
            }
            a if a.starts_with("--listen=") => {
                listen = a["--listen=".len()..].to_string();
                i += 1;
            }
            "--allow-host" if i + 1 < args.len() => {
                allow_hosts.push(args[i + 1].clone());
                i += 2;
            }
            a if a.starts_with("--allow-host=") => {
                allow_hosts.push(a["--allow-host=".len()..].to_string());
                i += 1;
            }
            _ => {
                i += 1;
            }
        }
    }
    if listen.trim().is_empty() {
        eprintln!("proxy listen address is required (--listen)");
        process::exit(1);
    }
    crate::proxy::run(&listen, allow_hosts);
}
