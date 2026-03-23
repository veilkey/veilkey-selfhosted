use std::process;
use veil_cli_rs::commands;
use veil_cli_rs::project_config::load_project_config;
use veil_cli_rs::state::state_dir;

static VERSION: &str = env!("CARGO_PKG_VERSION");

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

fn cmd_wrap_pty(args: &[String], api_url: &str, log_path: &str, patterns_file: Option<&str>) {
    #[cfg(unix)]
    {
        veil_cli_rs::pty::session::run(args, api_url, log_path, patterns_file);
    }
    #[cfg(not(unix))]
    {
        let _ = (args, api_url, log_path, patterns_file);
        eprintln!("wrap-pty is not supported on this platform");
        process::exit(1);
    }
}

fn main() {
    let _ = rustls::crypto::ring::default_provider().install_default();

    let raw_args: Vec<String> = std::env::args().collect();
    let api_url_opt = resolve_api_url();

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
        "wrap" => commands::cmd_wrap(&cmd_args, &api_url, &log_path, patterns_file.as_deref()),
        "proxy" => commands::cmd_proxy(&cmd_args),
        "wrap-pty" => cmd_wrap_pty(&cmd_args, &api_url, &log_path, patterns_file.as_deref()),
        "scan" => {
            let file = cmd_args.first().map(String::as_str).unwrap_or("-");
            commands::cmd_scan(
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
            commands::cmd_filter(file, &api_url, &log_path, patterns_file.as_deref());
        }
        "exec" => commands::cmd_exec(&cmd_args, &api_url),
        "resolve" => {
            if cmd_args.is_empty() {
                eprintln!("Usage: veilkey resolve <VK:hash>");
                process::exit(1);
            }
            commands::cmd_resolve(&cmd_args[0], &api_url);
        }
        "list" => commands::cmd_list(&log_path),
        "paste-mode" => commands::cmd_paste_mode(&cmd_args),
        "clear" => commands::cmd_clear(&log_path),
        "status" => commands::cmd_status(&api_url, &log_path, patterns_file.as_deref(), VERSION),
        "version" => println!("veilkey {}", VERSION),
        "help" | "-h" | "--help" => print_usage(),
        unknown => {
            eprintln!("Unknown command: {}", unknown);
            print_usage();
            process::exit(1);
        }
    }
}
