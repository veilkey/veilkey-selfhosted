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

pub fn cmd_exec(args: &[String], api_url: &str) {
    if args.is_empty() {
        eprintln!("Usage: veilkey exec <command...>");
        process::exit(1);
    }
    let client = VeilKeyClient::new(api_url);
    let vk_re = regex::Regex::new(crate::detector::VEILKEY_RE_STR).unwrap();

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
