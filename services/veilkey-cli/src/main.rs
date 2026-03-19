mod api;
mod config;
mod detector;
mod logger;
mod output;
mod project_config;
mod state;

use api::VeilKeyClient;
use config::load_config;
use detector::SecretDetector;
use logger::SessionLogger;
use output::{Finding, Formatter};
use project_config::load_project_config;
use state::{current_paste_mode, set_paste_mode, state_dir};
use std::io::{self, BufRead, Read};
use std::process;

static VERSION: &str = env!("CARGO_PKG_VERSION");

const SCAN_PREVIEW_LEN: usize = 8;

fn resolve_api_url() -> Option<String> {
    if let Ok(v) = std::env::var("VEILKEY_LOCALVAULT_URL") {
        if !v.is_empty() {
            return Some(v);
        }
    }
    if let Ok(v) = std::env::var("VEILKEY_API") {
        if !v.is_empty() {
            return Some(v);
        }
    }
    None
}

fn print_usage() {
    eprintln!(
        r#"Usage:
  veilkey scan [options] [file|-]   Scan file/stdin for secrets (detect only)
  veilkey filter [file|-]           Replace secrets in file/stdin (stdout)
  veilkey proxy [options]           Run the local egress proxy
  veilkey wrap <command...>         Run command + auto-replace secrets
  veilkey wrap-pty [command]        Interactive PTY + auto-replace (default: bash)
  veilkey exec <command...>         Resolve VK: hashes + run command
  veilkey resolve <VK:hash>         Resolve VK hash to original value
  veilkey list                      List detected VeilKey entries
  veilkey paste-mode [mode]         Get or set pasted temp issuance mode
  veilkey clear                     Clear session log
  veilkey status                    Show status
  veilkey version                   Show version

Options:
  --format <text|json|sarif>   Output format (default: text)
  --config <path>              Config file path (default: .veilkey.yml)
  --exit-code                  Exit with 1 if secrets found (for CI)
  --patterns <path>            Custom patterns file

Environment:
  VEILKEY_LOCALVAULT_URL       Preferred localvault URL
  VEILKEY_API                  Legacy endpoint variable (fallback)
  VEILKEY_STATE_DIR            State directory (default: $TMPDIR/veilkey-cli)
"#
    );
}

fn process_stream(detector: &mut SecretDetector, r: impl Read) {
    let reader = io::BufReader::new(r);
    for line in reader.lines() {
        match line {
            Ok(l) => println!("{}", detector.process_line(&l)),
            Err(_) => break,
        }
    }
}

fn cmd_wrap(args: &[String], api_url: &str, log_path: &str, patterns_file: Option<&str>) {
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

fn cmd_resolve(hash: &str, api_url: &str) {
    let client = VeilKeyClient::new(api_url);
    match client.resolve(hash) {
        Ok(v) => print!("{}", v),
        Err(e) => {
            eprintln!("ERROR: {}", e);
            process::exit(1);
        }
    }
}

fn cmd_exec(args: &[String], api_url: &str) {
    if args.is_empty() {
        eprintln!("Usage: veilkey exec <command...>");
        process::exit(1);
    }
    let client = VeilKeyClient::new(api_url);
    let vk_re = regex::Regex::new(detector::VEILKEY_RE_STR).unwrap();

    let resolved: Vec<String> = args
        .iter()
        .map(|arg| {
            vk_re
                .replace_all(arg, |caps: &regex::Captures| {
                    match client.resolve(&caps[0]) {
                        Ok(v) => v,
                        Err(e) => {
                            eprintln!("WARNING: resolve {} failed: {}", &caps[0], e);
                            caps[0].to_string()
                        }
                    }
                })
                .to_string()
        })
        .collect();

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

fn cmd_scan(
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

    let (reader, file_name): (Box<dyn Read>, String) = if file == "-" {
        (Box::new(io::stdin()), "<stdin>".to_string())
    } else {
        match std::fs::File::open(file) {
            Ok(f) => (Box::new(f), file.to_string()),
            Err(_) => {
                eprintln!("ERROR: file not found: {}", file);
                process::exit(1);
            }
        }
    };

    formatter.header();
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
            let preview = if det.value.len() > SCAN_PREVIEW_LEN {
                format!("{}***", &det.value[..SCAN_PREVIEW_LEN])
            } else {
                det.value.clone()
            };
            formatter.format_finding(Finding {
                file: file_name.clone(),
                line: line_num,
                pattern: det.pattern.clone(),
                confidence: det.confidence,
                r#match: preview,
            });
        }
    }

    formatter.format_summary(&detector.stats);
    formatter.footer();

    if exit_code_flag && detector.stats.detections > 0 {
        process::exit(1);
    }
}

fn cmd_filter(file: &str, api_url: &str, log_path: &str, patterns_file: Option<&str>) {
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
        eprintln!(
            "\n[veilkey] {} secret(s) detected and replaced",
            detector.stats.detections
        );
    }
}

fn cmd_list(log_path: &str) {
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

fn cmd_clear(log_path: &str) {
    let logger = SessionLogger::new(log_path);
    let _ = logger.clear();
    println!("Session log cleared");
}

fn cmd_paste_mode(args: &[String]) {
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

fn cmd_status(api_url: &str, log_path: &str, patterns_file: Option<&str>) {
    println!("\x1b[0;36m=== veilkey ===\x1b[0m");
    println!();
    println!("Version: {}", VERSION);
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

// ── Proxy ────────────────────────────────────────────────────────────────────

mod proxy {
    use std::collections::HashSet;
    use std::io::{self, Read, Write};
    use std::net::{TcpListener, TcpStream};
    use std::thread;
    use std::time::Duration;

    fn proxy_max_header() -> usize {
        std::env::var("VEILKEY_PROXY_MAX_HEADER")
            .ok()
            .and_then(|v| v.parse::<usize>().ok())
            .unwrap_or(16384)
    }

    fn proxy_timeout() -> Duration {
        let secs = std::env::var("VEILKEY_PROXY_TIMEOUT")
            .ok()
            .and_then(|v| v.parse::<u64>().ok())
            .unwrap_or(30);
        Duration::from_secs(secs)
    }

    fn default_https_port() -> String {
        std::env::var("VEILKEY_PROXY_DEFAULT_HTTPS_PORT").unwrap_or_else(|_| {
            eprintln!("error: VEILKEY_PROXY_DEFAULT_HTTPS_PORT is required");
            std::process::exit(1);
        })
    }

    fn default_http_port() -> String {
        std::env::var("VEILKEY_PROXY_DEFAULT_HTTP_PORT").unwrap_or_else(|_| {
            eprintln!("error: VEILKEY_PROXY_DEFAULT_HTTP_PORT is required");
            std::process::exit(1);
        })
    }

    pub fn run(listen: &str, allow_hosts: Vec<String>) {
        let listener = match TcpListener::bind(listen) {
            Ok(l) => l,
            Err(e) => {
                eprintln!("proxy listen failed: {}", e);
                std::process::exit(1);
            }
        };
        let allow_set: HashSet<String> =
            allow_hosts.into_iter().map(|h| h.to_lowercase()).collect();

        for stream in listener.incoming() {
            match stream {
                Ok(s) => {
                    let allow = allow_set.clone();
                    thread::spawn(move || {
                        if let Err(e) = handle_connection(s, &allow) {
                            eprintln!("proxy error: {}", e);
                        }
                    });
                }
                Err(e) => eprintln!("accept error: {}", e),
            }
        }
    }

    fn host_allowed(host: &str, allow_set: &HashSet<String>) -> bool {
        allow_set.is_empty() || allow_set.contains(&host.to_lowercase())
    }

    fn handle_connection(mut stream: TcpStream, allow_set: &HashSet<String>) -> io::Result<()> {
        stream.set_read_timeout(Some(proxy_timeout()))?;

        // Read HTTP request headers
        let mut buf = Vec::new();
        let mut tmp = [0u8; 1];
        loop {
            let n = stream.read(&mut tmp)?;
            if n == 0 {
                return Ok(());
            }
            buf.push(tmp[0]);
            if buf.ends_with(b"\r\n\r\n") {
                break;
            }
            if buf.len() > proxy_max_header() {
                return Ok(());
            }
        }

        let header_str = String::from_utf8_lossy(&buf).to_string();
        let mut lines = header_str.lines();
        let request_line = match lines.next() {
            Some(l) => l,
            None => return Ok(()),
        };
        let parts: Vec<&str> = request_line.splitn(3, ' ').collect();
        if parts.len() < 3 {
            return Ok(());
        }
        let method = parts[0];
        let target = parts[1];

        if method == "CONNECT" {
            // HTTPS CONNECT tunnel
            let host = target.split(':').next().unwrap_or(target);
            if !host_allowed(host, allow_set) {
                let _ = stream.write_all(b"HTTP/1.1 403 Forbidden\r\n\r\nhost not allowed");
                return Ok(());
            }
            let addr = if target.contains(':') {
                target.to_string()
            } else {
                format!("{}:{}", target, default_https_port())
            };
            match TcpStream::connect(&addr) {
                Err(_) => {
                    let _ = stream.write_all(b"HTTP/1.1 502 Bad Gateway\r\n\r\n");
                }
                Ok(upstream) => {
                    stream.write_all(b"HTTP/1.1 200 Connection Established\r\n\r\n")?;
                    let mut client_r = stream.try_clone()?;
                    let mut upstream_w = upstream.try_clone()?;
                    thread::spawn(move || {
                        let _ = io::copy(&mut client_r, &mut upstream_w);
                    });
                    let mut upstream_r = upstream;
                    let _ = io::copy(&mut upstream_r, &mut stream);
                }
            }
        } else {
            // Plain HTTP proxy
            let host = if target.starts_with("http") {
                target
                    .split("//")
                    .nth(1)
                    .and_then(|s| s.split('/').next())
                    .and_then(|s| s.split(':').next())
                    .unwrap_or("")
                    .to_string()
            } else {
                header_str
                    .lines()
                    .find(|l| l.to_lowercase().starts_with("host:"))
                    .and_then(|l| l.split_once(':').map(|(_, v)| v))
                    .map(|s| s.trim().split(':').next().unwrap_or("").to_string())
                    .unwrap_or_default()
            };

            if !host_allowed(&host, allow_set) {
                let _ = stream.write_all(b"HTTP/1.1 403 Forbidden\r\n\r\nhost not allowed");
                return Ok(());
            }

            let addr = if target.starts_with("http") {
                let hp = target
                    .split("//")
                    .nth(1)
                    .and_then(|s| s.split('/').next())
                    .unwrap_or("");
                if hp.contains(':') {
                    hp.to_string()
                } else {
                    format!("{}:{}", hp, default_http_port())
                }
            } else {
                format!("{}:{}", host, default_http_port())
            };

            match TcpStream::connect(&addr) {
                Err(_) => {
                    let _ = stream.write_all(b"HTTP/1.1 502 Bad Gateway\r\n\r\n");
                }
                Ok(mut upstream) => {
                    upstream.write_all(&buf)?;
                    let mut client_r = stream.try_clone()?;
                    let mut upstream_w = upstream.try_clone()?;
                    thread::spawn(move || {
                        let _ = io::copy(&mut client_r, &mut upstream_w);
                    });
                    let _ = io::copy(&mut upstream, &mut stream);
                }
            }
        }
        Ok(())
    }
}

fn cmd_proxy(args: &[String]) {
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
    proxy::run(&listen, allow_hosts);
}

// ── PTY wrap ─────────────────────────────────────────────────────────────────

#[cfg(unix)]
mod pty_wrap {
    use crate::{
        api::VeilKeyClient, config, detector::SecretDetector, logger::SessionLogger,
        state::state_dir,
    };
    use portable_pty::{native_pty_system, CommandBuilder, PtySize};
    use std::io::{self, BufRead, Write};

    pub fn cmd_wrap_pty(
        args: &[String],
        api_url: &str,
        log_path: &str,
        patterns_file: Option<&str>,
    ) {
        let shell_args: Vec<String> = if args.is_empty() {
            let shell = std::env::var("SHELL").unwrap_or_else(|_| "bash".to_string());
            vec![shell]
        } else {
            args.to_vec()
        };

        let cfg = match config::load_config(patterns_file) {
            Ok(c) => c,
            Err(e) => {
                eprintln!("ERROR: {}", e);
                std::process::exit(1);
            }
        };
        let client = VeilKeyClient::new(api_url);
        let logger = SessionLogger::new(log_path);

        // Save PID file
        let sd = state_dir();
        let _ = std::fs::create_dir_all(&sd);
        let pid_path = sd.join("guard.pid");
        let _ = std::fs::write(&pid_path, format!("{}", std::process::id()));

        let pty_system = native_pty_system();
        let pty_rows = std::env::var("LINES")
            .ok()
            .and_then(|v| v.parse::<u16>().ok())
            .unwrap_or(24);
        let pty_cols = std::env::var("COLUMNS")
            .ok()
            .and_then(|v| v.parse::<u16>().ok())
            .unwrap_or(80);
        let pair = match pty_system.openpty(PtySize {
            rows: pty_rows,
            cols: pty_cols,
            pixel_width: 0,
            pixel_height: 0,
        }) {
            Ok(p) => p,
            Err(e) => {
                eprintln!("ERROR: failed to open pty: {}", e);
                std::process::exit(1);
            }
        };

        // Resolve VK refs in environment variables → child sees real values
        let vk_re = regex::Regex::new(r"VK:(?:(?:TEMP|LOCAL|EXTERNAL):[0-9A-Fa-f]{4,64}|[0-9a-f]{8})").unwrap();
        let mut mask_map: Vec<(String, String)> = Vec::new(); // (plaintext, vk_ref)
        let mut cmd = CommandBuilder::new(&shell_args[0]);
        for arg in &shell_args[1..] {
            cmd.arg(arg);
        }
        for (key, value) in std::env::vars() {
            if vk_re.is_match(&value) {
                let resolved = vk_re.replace_all(&value, |caps: &regex::Captures| {
                    match client.resolve(&caps[0]) {
                        Ok(v) => {
                            mask_map.push((v.clone(), caps[0].to_string()));
                            v
                        }
                        Err(_) => caps[0].to_string(),
                    }
                }).to_string();
                cmd.env(key, resolved);
            }
        }
        // Sort mask_map by plaintext length descending (longest match first)
        mask_map.sort_by(|a, b| b.0.len().cmp(&a.0.len()));

        let mut child = match pair.slave.spawn_command(cmd) {
            Ok(c) => c,
            Err(e) => {
                eprintln!("ERROR: failed to spawn: {}", e);
                std::process::exit(1);
            }
        };
        drop(pair.slave);

        let master_reader = pair.master.try_clone_reader().expect("clone reader");
        let mut master_writer = pair.master.take_writer().expect("take writer");

        // stdin → pty
        std::thread::spawn(move || {
            let stdin = io::stdin();
            let mut stdin = stdin.lock();
            let _ = io::copy(&mut stdin, &mut master_writer);
        });

        // pty output → mask plaintext → stdout
        let stdout = io::stdout();
        let mut stdout = stdout.lock();

        let reader = io::BufReader::new(master_reader);
        for line in reader.lines() {
            match line {
                Ok(mut l) => {
                    // Replace any resolved plaintext with its VK ref
                    for (plaintext, vk_ref) in &mask_map {
                        if !plaintext.is_empty() {
                            l = l.replace(plaintext.as_str(), vk_ref.as_str());
                        }
                    }
                    let _ = stdout.write_all(l.as_bytes());
                    let _ = stdout.write_all(b"\n");
                }
                Err(_) => break,
            }
        }

        let _ = std::fs::remove_file(&pid_path);
        let exit_code = child
            .wait()
            .map(|s| if s.success() { 0 } else { 1 })
            .unwrap_or(1);

        if !mask_map.is_empty() {
            drop(stdout);
            eprintln!(
                "\n[veilkey] {} secret(s) masked in session",
                mask_map.len()
            );
        }
        std::process::exit(exit_code);
    }
}

#[cfg(not(unix))]
mod pty_wrap {
    pub fn cmd_wrap_pty(
        _args: &[String],
        _api_url: &str,
        _log_path: &str,
        _patterns_file: Option<&str>,
    ) {
        eprintln!("wrap-pty is not supported on this platform");
        std::process::exit(1);
    }
}

// ── main ─────────────────────────────────────────────────────────────────────

fn main() {
    let raw_args: Vec<String> = std::env::args().collect();
    let api_url_opt = resolve_api_url();

    // Commands that don't need an API endpoint
    const NO_API: &[&str] = &[
        "version",
        "help",
        "-h",
        "--help",
        "scan",
        "list",
        "clear",
        "status",
        "proxy",
        "paste-mode",
    ];

    let subcmd = raw_args.get(1).map(String::as_str).unwrap_or("");
    if api_url_opt.is_none() && !subcmd.is_empty() && !NO_API.contains(&subcmd) {
        eprintln!("ERROR: VeilKey endpoint URL is required.");
        eprintln!("  export VEILKEY_LOCALVAULT_URL=<localvault-url>");
        process::exit(1);
    }

    let sd = state_dir();
    let _ = std::fs::create_dir_all(&sd);
    let log_path = sd.join("session.log").to_string_lossy().to_string();

    // Parse global flags
    let mut patterns_file: Option<String> = None;
    let mut output_format: Option<String> = None;
    let mut config_path: Option<String> = None;
    let mut exit_code_flag = false;
    let mut cleaned: Vec<String> = Vec::new();

    let args = &raw_args[1..];
    let mut i = 0;
    while i < args.len() {
        let a = &args[i];
        if a == "--patterns" && i + 1 < args.len() {
            patterns_file = Some(args[i + 1].clone());
            i += 2;
        } else if let Some(v) = a.strip_prefix("--patterns=") {
            patterns_file = Some(v.to_string());
            i += 1;
        } else if a == "--format" && i + 1 < args.len() {
            output_format = Some(args[i + 1].clone());
            i += 2;
        } else if let Some(v) = a.strip_prefix("--format=") {
            output_format = Some(v.to_string());
            i += 1;
        } else if a == "--config" && i + 1 < args.len() {
            config_path = Some(args[i + 1].clone());
            i += 2;
        } else if let Some(v) = a.strip_prefix("--config=") {
            config_path = Some(v.to_string());
            i += 1;
        } else if a == "--exit-code" {
            exit_code_flag = true;
            i += 1;
        } else {
            cleaned.push(a.clone());
            i += 1;
        }
    }

    // Load project config; CLI flags take precedence
    if let Some(proj) = load_project_config(config_path.as_deref()) {
        if patterns_file.is_none() && !proj.patterns_file.is_empty() {
            patterns_file = Some(proj.patterns_file);
        }
        if output_format.is_none() && !proj.format.is_empty() {
            output_format = Some(proj.format);
        }
        if !exit_code_flag && proj.exit_code {
            exit_code_flag = true;
        }
    }

    let output_format = output_format.unwrap_or_else(|| "text".to_string());
    let api_url = api_url_opt.unwrap_or_default();

    if cleaned.is_empty() {
        print_usage();
        process::exit(1);
    }

    let cmd = cleaned[0].clone();
    let cmd_args = cleaned[1..].to_vec();

    match cmd.as_str() {
        "wrap" => cmd_wrap(&cmd_args, &api_url, &log_path, patterns_file.as_deref()),
        "proxy" => cmd_proxy(&cmd_args),
        "wrap-pty" => {
            pty_wrap::cmd_wrap_pty(&cmd_args, &api_url, &log_path, patterns_file.as_deref())
        }
        "scan" => {
            let file = cmd_args.first().map(String::as_str).unwrap_or("-");
            cmd_scan(
                file,
                &api_url,
                &log_path,
                patterns_file.as_deref(),
                &output_format,
                exit_code_flag,
            );
        }
        "filter" => {
            let file = cmd_args.first().map(String::as_str).unwrap_or("-");
            cmd_filter(file, &api_url, &log_path, patterns_file.as_deref());
        }
        "exec" => cmd_exec(&cmd_args, &api_url),
        "resolve" => {
            if cmd_args.is_empty() {
                eprintln!("Usage: veilkey resolve <VK:hash>");
                process::exit(1);
            }
            cmd_resolve(&cmd_args[0], &api_url);
        }
        "list" => cmd_list(&log_path),
        "paste-mode" => cmd_paste_mode(&cmd_args),
        "clear" => cmd_clear(&log_path),
        "status" => cmd_status(&api_url, &log_path, patterns_file.as_deref()),
        "version" => println!("veilkey {}", VERSION),
        "help" | "-h" | "--help" => print_usage(),
        unknown => {
            eprintln!("Unknown command: {}", unknown);
            print_usage();
            process::exit(1);
        }
    }
}
