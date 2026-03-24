use std::env;
use std::process;

fn find_bin(env_key: &str, name: &str) -> String {
    if let Ok(v) = env::var(env_key) {
        if !v.is_empty() {
            return v;
        }
    }
    // Try PATH lookup
    which(name).unwrap_or_else(|| {
        eprintln!("veil: {} not found (set {} or add to PATH)", name, env_key);
        process::exit(1);
    })
}

fn which(name: &str) -> Option<String> {
    let path = env::var("PATH").unwrap_or_default();
    for dir in path.split(':') {
        let candidate = format!("{}/{}", dir, name);
        if std::path::Path::new(&candidate).exists() {
            return Some(candidate);
        }
    }
    None
}

fn exec_replace(bin: &str, args: &[String]) -> ! {
    use std::os::unix::process::CommandExt;
    let err = process::Command::new(bin).args(args).exec();
    eprintln!("veil: exec {}: {}", bin, err);
    process::exit(1);
}

/// Find and load .veilkey/env
/// Priority: VEILKEY_ENV > walk up from cwd > ~/.veilkey/env > ~/veilkey-selfhosted/.veilkey/env
fn auto_load_env() {
    // 1. Explicit path
    if let Ok(p) = env::var("VEILKEY_ENV") {
        if !p.is_empty() && std::path::Path::new(&p).exists() {
            load_env_file(&p);
            return;
        }
    }

    // 2. Walk up from current dir
    let mut dir = env::current_dir().ok();
    while let Some(d) = &dir {
        let env_file = d.join(".veilkey").join("env");
        if env_file.exists() {
            load_env_file(&env_file.to_string_lossy());
            return;
        }
        dir = d.parent().map(|p| p.to_path_buf());
    }

    // 3. Home fallback paths
    if let Ok(home) = env::var("HOME") {
        for sub in &[".veilkey/env", "veilkey-selfhosted/.veilkey/env"] {
            let p = format!("{}/{}", home, sub);
            if std::path::Path::new(&p).exists() {
                load_env_file(&p);
                return;
            }
        }
    }
}

fn load_env_file(path: &str) {
    if let Ok(content) = std::fs::read_to_string(path) {
        for line in content.lines() {
            let line = line.trim();
            if line.starts_with('#') || line.is_empty() || line.starts_with("#!/") {
                continue;
            }
            let line = line.strip_prefix("export ").unwrap_or(line);
            if let Some((k, v)) = line.split_once('=') {
                let k = k.trim();
                let v = v.trim().trim_matches('"').trim_matches('\'');
                env::set_var(k, v);
            }
        }
    }
}

fn main() {
    // Auto-load .veilkey/env from current or parent dirs
    // Clear legacy env vars first so .veilkey/env takes precedence
    auto_load_env();
    // If VEILKEY_LOCALVAULT_URL is set, ensure VEILKEY_API matches
    if let Ok(url) = env::var("VEILKEY_LOCALVAULT_URL") {
        env::set_var("VEILKEY_API", &url);
    }

    let cli_bin = find_bin("VEILKEY_CLI_BIN", "veilkey-cli");

    env::set_var("VEIL_PS1", "(veil) ");

    let args: Vec<String> = env::args().skip(1).collect();

    if args.is_empty() {
        // Create temp rcfile for custom prompt
        let rc_path = format!("{}/veil-bashrc", env::temp_dir().display());
        let hist_path = format!(
            "{}/veil-history.{}",
            env::temp_dir().display(),
            std::process::id()
        );
        let rc_content = format!(
            r#"
[ -f ~/.bashrc ] && source ~/.bashrc
[ -f ~/.bash_profile ] && source ~/.bash_profile
export PS1="\[\033[36m\](VEIL)\[\033[0m\] \h:\W \u\$ "
# Session-only history: works within session, deleted on exit
export HISTFILE="{}"
export HISTSIZE=500
export HISTFILESIZE=0
export HISTCONTROL=ignoreboth:erasedups
trap 'rm -f "$HISTFILE"' EXIT
"#,
            hist_path
        );
        let _ = std::fs::write(&rc_path, rc_content);

        // veil → enter protected shell with custom prompt
        exec_replace(
            &cli_bin,
            &[
                "wrap-pty".to_string(),
                "bash".to_string(),
                "--rcfile".to_string(),
                rc_path,
            ],
        );
    }

    match args[0].as_str() {
        "status" => {
            exec_replace(&cli_bin, &["status".to_string()]);
        }
        "exec" => {
            let mut full = vec!["exec".to_string()];
            full.extend_from_slice(&args[1..]);
            exec_replace(&cli_bin, &full);
        }
        "scan" => {
            let mut full = vec!["scan".to_string()];
            full.extend_from_slice(&args[1..]);
            exec_replace(&cli_bin, &full);
        }
        "localvault" => {
            let sub = args.get(1).map(|s| s.as_str()).unwrap_or("init");
            match sub {
                "init" | "update" => {
                    let center_url = env::var("VEILKEY_VAULTCENTER_URL")
                        .or_else(|_| env::var("VEILKEY_KEYCENTER_URL"))
                        .unwrap_or_default();
                    let lv_url = env::var("VEILKEY_LOCALVAULT_URL").unwrap_or_default();
                    // Extract port from LOCALVAULT_URL if set, else default
                    let port = lv_url
                        .rsplit(':')
                        .next()
                        .and_then(|p| p.trim_end_matches('/').parse::<u16>().ok())
                        .unwrap_or(10180);

                    if center_url.is_empty() {
                        eprintln!("veil localvault: VEILKEY_VAULTCENTER_URL is not set");
                        eprintln!("  Set it in your shell or ~/.veilkey.yml");
                        process::exit(1);
                    }

                    let gist_url = "https://gist.githubusercontent.com/dalsoop/11e00346263678340189cdfdc79644b5/raw/install-localvault.sh";
                    let script = format!(
                        "VEILKEY_CENTER_URL='{}' VEILKEY_PORT='{}' curl -sL '{}?{}' | bash",
                        center_url,
                        port,
                        gist_url,
                        std::time::SystemTime::now()
                            .duration_since(std::time::UNIX_EPOCH)
                            .unwrap_or_default()
                            .as_secs()
                    );
                    let status = process::Command::new("bash")
                        .arg("-c")
                        .arg(&script)
                        .status()
                        .unwrap_or_else(|e| {
                            eprintln!("veil localvault: failed to run installer: {}", e);
                            process::exit(1);
                        });
                    process::exit(status.code().unwrap_or(1));
                }
                "stop" => {
                    let pid_file = format!(
                        "{}/.localvault/localvault.pid",
                        env::current_dir().unwrap().display()
                    );
                    match std::fs::read_to_string(&pid_file) {
                        Ok(pid) => {
                            let pid = pid.trim();
                            let _ = process::Command::new("kill").arg(pid).status();
                            let _ = std::fs::remove_file(&pid_file);
                            println!("localvault stopped (PID: {})", pid);
                        }
                        Err(_) => eprintln!("localvault is not running (no pid file)"),
                    }
                }
                "log" | "logs" => {
                    let log_file = format!(
                        "{}/.localvault/localvault.log",
                        env::current_dir().unwrap().display()
                    );
                    exec_replace("tail", &["-f".to_string(), log_file]);
                }
                "status" => {
                    let lv_url = env::var("VEILKEY_LOCALVAULT_URL")
                        .unwrap_or_else(|_| "http://localhost:10180".to_string());
                    let health_url = format!("{}/health", lv_url.trim_end_matches('/'));
                    let status = process::Command::new("curl")
                        .args(["-s", &health_url])
                        .status();
                    if status.map(|s| s.success()).unwrap_or(false) {
                        println!();
                    } else {
                        eprintln!("localvault is not reachable at {}", health_url);
                        process::exit(1);
                    }
                }
                _ => {
                    eprintln!("Usage:");
                    eprintln!("  veil localvault [init]    Install/update localvault in current directory");
                    eprintln!("  veil localvault stop      Stop localvault");
                    eprintln!("  veil localvault log       Tail localvault logs");
                    eprintln!("  veil localvault status    Check localvault health");
                    process::exit(1);
                }
            }
        }
        "help" | "-h" | "--help" => {
            println!("Usage:");
            println!("  veil                     Enter protected session (PTY masking)");
            println!("  veil status              Show VeilKey connection status");
            println!("  veil exec <command...>   Resolve VK refs in args and execute");
            println!("  veil scan [file...]      Scan files for secrets");
            println!("  veil localvault [init]   Install/update localvault here");
            println!("  veil localvault stop     Stop localvault");
            println!("  veil localvault log      Tail logs");
            println!("  veil localvault status   Health check");
            println!("  veil help                Show this help");
        }
        _ => {
            // Pass through to wrap-pty with args as command
            let mut full = vec!["wrap-pty".to_string()];
            full.extend_from_slice(&args);
            exec_replace(&cli_bin, &full);
        }
    }
}
