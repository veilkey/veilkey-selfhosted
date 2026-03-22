use std::io::{self, BufRead, Read};
use std::process;
use veil_cli_rs::api::VeilKeyClient;
use veil_cli_rs::config::load_config;
use veil_cli_rs::detector::SecretDetector;
use veil_cli_rs::logger::SessionLogger;
use veil_cli_rs::output::{Finding, Formatter};
use veil_cli_rs::project_config::load_project_config;
use veil_cli_rs::state::{current_paste_mode, set_paste_mode, state_dir};

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
    let vk_re = regex::Regex::new(veil_cli_rs::detector::VEILKEY_RE_STR).unwrap();

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
        // Check if any replacements used VK:TEMP refs
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
    use nix::libc;
    use nix::sys::signal::{self, SigHandler, Signal};
    use nix::unistd::{execvp, fork, ForkResult};
    use std::ffi::CString;
    use std::io::{self};
    use std::os::fd::{AsRawFd, RawFd};
    use std::sync::{Arc, Mutex};
    use std::time::Duration;
    use veil_cli_rs::{
        api::VeilKeyClient,
        config::{load_config, CompiledPattern},
        state::state_dir,
    };

    static mut MASTER_FD: RawFd = -1;

    extern "C" fn handle_sigwinch(_: libc::c_int) {
        unsafe {
            if MASTER_FD >= 0 {
                let mut ws: libc::winsize = std::mem::zeroed();
                libc::ioctl(0, libc::TIOCGWINSZ, &mut ws);
                libc::ioctl(MASTER_FD, libc::TIOCSWINSZ, &ws);
            }
        }
    }

    const CYAN: &str = "\x1b[36m";
    const RED: &str = "\x1b[31m";
    const RESET: &str = "\x1b[0m";
    const BOLD: &str = "\x1b[1m";

    fn colorize_ref(vk_ref: &str) -> String {
        if vk_ref.contains(":LOCAL:") {
            format!("{}{}{}{}", BOLD, CYAN, vk_ref, RESET)
        } else if vk_ref.contains(":TEMP:") {
            format!("{}{}{}{}", BOLD, RED, vk_ref, RESET)
        } else {
            vk_ref.to_string()
        }
    }

    /// Colorize a VK ref and pad to match original secret length (prevents cursor shift)
    fn padded_colorize_ref(vk_ref: &str, original_len: usize) -> String {
        let colored = colorize_ref(vk_ref);
        let visible_len = vk_ref.len();
        if visible_len < original_len {
            format!("{}{}", colored, " ".repeat(original_len - visible_len))
        } else {
            colored
        }
    }

    fn mask_output(
        data: &[u8],
        mask_map: &[(String, String)],
        patterns: &[CompiledPattern],
        client: &VeilKeyClient,
        recent_input: &str,
    ) -> Vec<u8> {
        let mut s = String::from_utf8_lossy(data).to_string();

        // 1. Known secrets from mask_map — padded to same visible length
        for (plaintext, vk_ref) in mask_map {
            if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
                s = s.replace(
                    plaintext.as_str(),
                    &padded_colorize_ref(vk_ref, plaintext.len()),
                );
            }
        }

        // 2. Pattern-detected secrets — scan, register, replace with padding
        //    Skip matches that are echo-back of recent user input (prevents false positives
        //    from commands the user just typed, e.g. `export TOKEN=...`)
        let scan_copy = s.clone();
        for pat in patterns {
            for caps in pat.regex.captures_iter(&scan_copy) {
                let m = caps
                    .get(pat.group.max(1))
                    .or_else(|| caps.get(1))
                    .unwrap_or_else(|| caps.get(0).unwrap());
                let secret = m.as_str().trim_end_matches(['\r', '\n']);
                if secret.len() < 8 || secret.starts_with("VK:") {
                    continue;
                }
                if mask_map.iter().any(|(p, _)| p == secret) {
                    continue;
                }
                // Skip if the matched secret is part of recent user input (echo-back).
                // The shell echoes typed commands; masking those would be a false positive.
                if !recent_input.is_empty() && recent_input.contains(secret) {
                    continue;
                }
                match client.issue(secret) {
                    Ok(ref_canonical) => {
                        s = s.replace(secret, &padded_colorize_ref(&ref_canonical, secret.len()));
                    }
                    Err(e) => {
                        eprintln!(
                            "[veilkey] issue failed for pattern {}: {} — redacting (fail-closed)",
                            pat.name, e
                        );
                        let redacted = format!("[REDACTED:{}]", pat.name);
                        s = s.replace(secret, &padded_colorize_ref(&redacted, secret.len()));
                    }
                }
            }
        }

        s.into_bytes()
    }

    pub fn cmd_wrap_pty(
        args: &[String],
        api_url: &str,
        _log_path: &str,
        _patterns_file: Option<&str>,
    ) {
        let shell_args: Vec<String> = if args.is_empty() {
            let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/bash".to_string());
            vec![shell]
        } else {
            args.to_vec()
        };

        let client = VeilKeyClient::new(api_url);

        // Load detection patterns for outbound scanning
        let cfg = load_config(None).ok();
        let patterns: Vec<CompiledPattern> = cfg.map(|c| c.patterns).unwrap_or_default();

        // Save PID file
        let sd = state_dir();
        let _ = std::fs::create_dir_all(&sd);
        let pid_path = sd.join("guard.pid");
        let _ = std::fs::write(&pid_path, format!("{}", std::process::id()));

        // 1. Fetch all secrets from VaultCenter → build mask_map
        //    fail-closed: if API is unreachable, refuse to start
        let mut mask_map: Vec<(String, String)> = match client.fetch_all_secrets_mask_map() {
            Some(map) => map,
            None => {
                eprintln!("[veilkey] FATAL: cannot build mask map — refusing to start shell (fail-closed)");
                eprintln!("[veilkey] ensure VaultCenter is reachable and try again");
                std::process::exit(1);
            }
        };
        eprintln!("[veilkey] loaded {} secret(s) from vaults", mask_map.len());

        // 2. Also resolve VK refs in environment variables
        let vk_re = regex::Regex::new(veil_cli_rs::detector::VEILKEY_RE_STR).unwrap();
        let mut child_env: Vec<(String, String)> = Vec::new();
        for (key, value) in std::env::vars() {
            if vk_re.is_match(&value) {
                let resolved = vk_re
                    .replace_all(&value, |caps: &regex::Captures| {
                        match client.resolve(&caps[0]) {
                            Ok(v) => {
                                if !mask_map.iter().any(|(p, _)| p == &v) {
                                    mask_map.push((v.clone(), caps[0].to_string()));
                                }
                                v
                            }
                            Err(_) => caps[0].to_string(),
                        }
                    })
                    .to_string();
                child_env.push((key, resolved));
            }
        }
        mask_map.sort_by(|a, b| b.0.len().cmp(&a.0.len()));

        // Open PTY via libc
        let (master_fd, slave_fd): (RawFd, RawFd) = unsafe {
            let mut master: RawFd = 0;
            let mut slave: RawFd = 0;
            if libc::openpty(
                &mut master,
                &mut slave,
                std::ptr::null_mut(),
                std::ptr::null_mut(),
                std::ptr::null_mut(),
            ) != 0
            {
                eprintln!("ERROR: failed to open pty");
                std::process::exit(1);
            }
            (master, slave)
        };

        // SIGWINCH handler
        unsafe {
            MASTER_FD = master_fd;
            signal::signal(Signal::SIGWINCH, SigHandler::Handler(handle_sigwinch)).ok();
            // Set initial size
            let mut ws: libc::winsize = std::mem::zeroed();
            libc::ioctl(0, libc::TIOCGWINSZ, &mut ws);
            libc::ioctl(master_fd, libc::TIOCSWINSZ, &ws);
        }

        // Fork
        match unsafe { fork() }.expect("fork failed") {
            ForkResult::Child => {
                unsafe {
                    libc::close(master_fd);
                }
                unsafe {
                    libc::setsid();
                }
                unsafe {
                    libc::ioctl(slave_fd, libc::TIOCSCTTY as _, 0);
                }

                unsafe {
                    libc::dup2(slave_fd, 0);
                }
                unsafe {
                    libc::dup2(slave_fd, 1);
                }
                unsafe {
                    libc::dup2(slave_fd, 2);
                }
                if slave_fd > 2 {
                    unsafe {
                        libc::close(slave_fd);
                    }
                }

                // Block /proc/self/environ reads BEFORE setting resolved env vars.
                // PR_SET_DUMPABLE=0 prevents even root from reading /proc/{pid}/environ
                // for env vars set after this point.
                unsafe {
                    libc::prctl(libc::PR_SET_DUMPABLE, 0);
                }

                // Set resolved env vars (plaintext — protected by prctl above)
                for (key, value) in &child_env {
                    std::env::set_var(key, value);
                }

                let prog = CString::new(shell_args[0].as_str()).unwrap();
                let c_args: Vec<CString> = shell_args
                    .iter()
                    .map(|a| CString::new(a.as_str()).unwrap())
                    .collect();
                let exec_result = execvp(&prog, &c_args);
                // execvp only returns on error
                eprintln!(
                    "[veilkey] execvp failed: {} ({:?})",
                    shell_args[0], exec_result
                );
                std::process::exit(1);
            }
            ForkResult::Parent { child } => {
                unsafe {
                    libc::close(slave_fd);
                }

                // Save stdin termios and switch to raw mode
                let stdin_fd = io::stdin().as_raw_fd();
                let mut old_termios: libc::termios = unsafe { std::mem::zeroed() };
                let has_termios = unsafe { libc::tcgetattr(stdin_fd, &mut old_termios) } == 0;
                if has_termios {
                    let mut raw = old_termios;
                    unsafe {
                        libc::cfmakeraw(&mut raw);
                    }
                    unsafe {
                        libc::tcsetattr(stdin_fd, libc::TCSANOW, &raw);
                    }
                }

                let mask_map = Arc::new(std::sync::RwLock::new(mask_map));
                let patterns = Arc::new(patterns);
                let client = Arc::new(client);

                // Background mask_map sync thread — long polls /api/mask-map for changes
                {
                    let sync_map = mask_map.clone();
                    let sync_client = client.clone();
                    std::thread::spawn(move || {
                        let mut version: u64 = 0;
                        loop {
                            let url = format!(
                                "{}/api/mask-map?version={}&wait=30",
                                sync_client.base_url(),
                                version
                            );
                            match sync_client.raw_get(&url) {
                                Ok(resp) => {
                                    let data: serde_json::Value =
                                        resp.into_json().unwrap_or_default();
                                    let new_version = data["version"].as_u64().unwrap_or(version);
                                    let changed = data["changed"].as_bool().unwrap_or(false);
                                    if changed && new_version > version {
                                        if let Some(entries) = data["entries"].as_array() {
                                            let mut new_map: Vec<(String, String)> = Vec::new();
                                            for e in entries {
                                                let r = e["ref"].as_str().unwrap_or_default();
                                                let v = e["value"].as_str().unwrap_or_default();
                                                let trimmed = v.trim_end_matches(['\r', '\n']);
                                                if !trimmed.is_empty() && !r.is_empty() {
                                                    new_map
                                                        .push((trimmed.to_string(), r.to_string()));
                                                }
                                            }
                                            // Add encoded variants (base64, hex)
                                            use base64::Engine as _;
                                            let mut encoded: Vec<(String, String)> = Vec::new();
                                            for (pt, vr) in &new_map {
                                                if pt.len() < 8 {
                                                    continue;
                                                }
                                                let b64 = base64::engine::general_purpose::STANDARD
                                                    .encode(pt.as_bytes());
                                                if !new_map.iter().any(|(p, _)| p == &b64) {
                                                    encoded.push((b64, vr.clone()));
                                                }
                                                let hex: String = pt
                                                    .bytes()
                                                    .map(|b| format!("{:02x}", b))
                                                    .collect();
                                                if !new_map.iter().any(|(p, _)| p == &hex) {
                                                    encoded.push((hex, vr.clone()));
                                                }
                                            }
                                            new_map.extend(encoded);
                                            new_map.sort_by(|a, b| b.0.len().cmp(&a.0.len()));
                                            // Remove entries where plaintext is substring of VK ref
                                            let all_refs: Vec<String> =
                                                new_map.iter().map(|(_, r)| r.clone()).collect();
                                            new_map.retain(|(pt, _)| {
                                                !all_refs
                                                    .iter()
                                                    .any(|r| r.contains(pt.as_str()) && r != pt)
                                            });
                                            if let Ok(mut map) = sync_map.write() {
                                                *map = new_map;
                                                eprintln!(
                                                    "[veilkey] mask_map synced: {} secret(s) (v{})",
                                                    map.len(),
                                                    new_version
                                                );
                                            }
                                        }
                                        version = new_version;
                                    } else {
                                        version = new_version;
                                    }
                                }
                                Err(_) => {
                                    // API unreachable — keep current mask_map, retry after delay
                                    std::thread::sleep(Duration::from_secs(10));
                                }
                            }
                        }
                    });
                }

                // Track recent stdin input to skip echo-back masking
                let recent_input = Arc::new(Mutex::new(String::new()));

                // stdin → master (forward input to PTY)
                let master_wr = master_fd;
                let input_tracker = recent_input.clone();
                std::thread::spawn(move || {
                    let mut buf = [0u8; 4096];
                    loop {
                        let n = unsafe { libc::read(stdin_fd, buf.as_mut_ptr() as _, buf.len()) };
                        if n <= 0 {
                            break;
                        }
                        // Track what was typed
                        if let Ok(s) = std::str::from_utf8(&buf[..n as usize]) {
                            let mut tracker = input_tracker.lock().unwrap();
                            tracker.push_str(s);
                            // Keep only last 4KB
                            if tracker.len() > 4096 {
                                let start = tracker.len() - 4096;
                                *tracker = tracker[start..].to_string();
                            }
                        }
                        unsafe {
                            libc::write(master_wr, buf.as_ptr() as _, n as _);
                        }
                    }
                });

                // master → stdout (filter output from PTY)
                //
                // Sliding-window design: keep an overlap buffer from the tail of the
                // previous flush so that secrets spanning two reads are still caught.
                // The overlap length equals the longest secret in mask_map.
                let mask = mask_map.clone();
                let input_ref = recent_input.clone();
                let stdout_fd = io::stdout().as_raw_fd();
                let mut partial_buf: Vec<u8> = Vec::new();

                let max_secret_len = mask
                    .read()
                    .unwrap()
                    .iter()
                    .map(|(p, _)| p.len())
                    .max()
                    .unwrap_or(0);
                // overlap_buf holds the tail of the last flushed output (up to max_secret_len bytes).
                // It is prepended to the next batch so cross-boundary secrets are matched.
                let mut overlap_buf: Vec<u8> = Vec::new();

                let lookahead_ms: u64 = std::env::var("VEILKEY_LOOKAHEAD_MS")
                    .ok()
                    .and_then(|v| v.parse().ok())
                    .unwrap_or(50);

                let mut buf = [0u8; 32768];
                loop {
                    let n = unsafe { libc::read(master_fd, buf.as_mut_ptr() as _, buf.len()) };
                    if n <= 0 {
                        break;
                    }
                    let n = n as usize;
                    let chunk = &buf[..n];
                    if let Some(last_nl) = chunk.iter().rposition(|&b| b == b'\n') {
                        // Prepend overlap from previous flush to catch cross-boundary secrets
                        let mut combined = std::mem::take(&mut overlap_buf);
                        combined.extend_from_slice(&partial_buf);
                        combined.extend_from_slice(&chunk[..last_nl + 1]);
                        let overlap_len = overlap_buf.len(); // was taken, so 0 now — use saved value
                        let saved_overlap_len = combined.len().min(max_secret_len);

                        let ri = input_ref.lock().unwrap().clone();
                        let masked =
                            mask_output(&combined, &mask.read().unwrap(), &patterns, &client, &ri);

                        // Only write the NEW portion (skip the overlap that was already written)
                        if masked.len() > overlap_len {
                            let new_output = &masked[overlap_len..];
                            unsafe {
                                libc::write(stdout_fd, new_output.as_ptr() as _, new_output.len());
                            }
                        }

                        // Save tail as overlap for next iteration
                        overlap_buf =
                            combined[combined.len().saturating_sub(saved_overlap_len)..].to_vec();
                        partial_buf.clear();
                        if last_nl + 1 < n {
                            partial_buf.extend_from_slice(&chunk[last_nl + 1..]);
                        }
                    } else {
                        partial_buf.extend_from_slice(chunk);
                        if !partial_buf.is_empty() {
                            std::thread::sleep(Duration::from_millis(30));
                            let mut peek = [0u8; 1];
                            unsafe {
                                let flags = libc::fcntl(master_fd, libc::F_GETFL);
                                libc::fcntl(master_fd, libc::F_SETFL, flags | libc::O_NONBLOCK);
                                let peek_result = libc::read(master_fd, peek.as_mut_ptr() as _, 1);
                                libc::fcntl(master_fd, libc::F_SETFL, flags);
                                if peek_result <= 0 {
                                    // Check if partial_buf ends with prefix of any known secret
                                    // (removed the >8 length gate — all secrets now checked)
                                    let buf_str = String::from_utf8_lossy(&partial_buf);
                                    let has_partial_secret =
                                        mask.read().unwrap().iter().any(|(plaintext, _)| {
                                            let pl = plaintext.as_str();
                                            (4..pl.len()).any(|i| buf_str.ends_with(&pl[..i]))
                                        });
                                    if has_partial_secret {
                                        // Wait for the rest of the secret
                                        std::thread::sleep(Duration::from_millis(lookahead_ms));
                                        let n2 =
                                            libc::read(master_fd, buf.as_mut_ptr() as _, buf.len());
                                        if n2 > 0 {
                                            partial_buf.extend_from_slice(&buf[..n2 as usize]);
                                            continue; // Re-enter loop to process combined buffer
                                        }
                                    }
                                    // Flush partial with overlap
                                    let mut combined = std::mem::take(&mut overlap_buf);
                                    let prev_overlap_len = combined.len();
                                    combined.extend_from_slice(&partial_buf);
                                    let saved_overlap_len = combined.len().min(max_secret_len);

                                    let ri = input_ref.lock().unwrap().clone();
                                    let masked = mask_output(
                                        &combined,
                                        &mask.read().unwrap(),
                                        &patterns,
                                        &client,
                                        &ri,
                                    );
                                    if masked.len() > prev_overlap_len {
                                        let new_output = &masked[prev_overlap_len..];
                                        libc::write(
                                            stdout_fd,
                                            new_output.as_ptr() as _,
                                            new_output.len(),
                                        );
                                    }
                                    overlap_buf = combined
                                        [combined.len().saturating_sub(saved_overlap_len)..]
                                        .to_vec();
                                    partial_buf.clear();
                                } else {
                                    partial_buf.push(peek[0]);
                                }
                            }
                        }
                    }
                }

                // Flush remaining
                if !partial_buf.is_empty() || !overlap_buf.is_empty() {
                    let mut combined = overlap_buf;
                    let prev_overlap_len = combined.len();
                    combined.extend_from_slice(&partial_buf);
                    let ri = input_ref.lock().unwrap().clone();
                    let masked =
                        mask_output(&combined, &mask.read().unwrap(), &patterns, &client, &ri);
                    if masked.len() > prev_overlap_len {
                        let new_output = &masked[prev_overlap_len..];
                        unsafe {
                            libc::write(stdout_fd, new_output.as_ptr() as _, new_output.len());
                        }
                    }
                }

                // Restore terminal
                if has_termios {
                    unsafe {
                        libc::tcsetattr(stdin_fd, libc::TCSANOW, &old_termios);
                    }
                }

                let _ = std::fs::remove_file(&pid_path);

                // Wait for child
                let mut status: libc::c_int = 0;
                unsafe {
                    libc::waitpid(child.as_raw(), &mut status, 0);
                }
                let exit_code = if libc::WIFEXITED(status) {
                    libc::WEXITSTATUS(status)
                } else {
                    1
                };

                if let Ok(m) = mask.read() {
                    if !m.is_empty() {
                        eprintln!("\n[veilkey] {} secret(s) masked in session", m.len());
                    }
                }
                std::process::exit(exit_code);
            }
        }
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
    // Install rustls crypto provider before any TLS operations
    let _ = rustls::crypto::ring::default_provider().install_default();

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
