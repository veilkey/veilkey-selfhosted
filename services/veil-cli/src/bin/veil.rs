use std::env;
use std::io::Write;
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

/// Load only operator-selected configuration, never files discovered in a project.
fn auto_load_env() -> std::io::Result<()> {
    if let Ok(path) = env::var("VEILKEY_ENV") {
        if !path.is_empty() {
            return load_env_file(std::path::Path::new(&path));
        }
    }
    if let Ok(home) = env::var("HOME") {
        let path = std::path::Path::new(&home).join(".veilkey/env");
        if path.exists() {
            return load_env_file(&path);
        }
    }
    Ok(())
}

fn load_env_file(path: &std::path::Path) -> std::io::Result<()> {
    let content = std::fs::read_to_string(path)?;
    for line in content.lines() {
        let line = line.trim();
        if line.starts_with('#') || line.is_empty() {
            continue;
        }
        let line = line.strip_prefix("export ").unwrap_or(line);
        if let Some((key, value)) = line.split_once('=') {
            let key = key.trim();
            if key.is_empty()
                || !key.bytes().enumerate().all(|(i, c)| {
                    c == b'_' || c.is_ascii_alphabetic() || (i > 0 && c.is_ascii_digit())
                })
            {
                continue;
            }
            env::set_var(key, value.trim().trim_matches('"').trim_matches('\''));
        }
    }
    Ok(())
}

fn shell_rcfile() -> std::io::Result<tempfile::NamedTempFile> {
    let mut file = tempfile::Builder::new().prefix("veil-bashrc-").tempfile()?;
    file.write_all(
        br#"
[ -f ~/.bashrc ] && source ~/.bashrc
[ -f ~/.bash_profile ] && source ~/.bash_profile
export PS1="\[\033[36m\](VEIL)\[\033[0m\] \h:\W \u\$ "
# Keep command history in memory for this protected session only.
unset HISTFILE
"#,
    )?;
    file.flush()?;
    Ok(file)
}

fn find_repo_install_script() -> Option<String> {
    let cwd = env::current_dir().ok()?;
    for dir in cwd.ancestors() {
        let common = dir
            .join("install")
            .join("common")
            .join("install-localvault.sh");
        if common.exists() {
            return Some(common.to_string_lossy().to_string());
        }
        let proxmox = dir
            .join("install")
            .join("proxmox-lxc-debian")
            .join("install-localvault.sh");
        if proxmox.exists() {
            return Some(proxmox.to_string_lossy().to_string());
        }
    }
    None
}

fn main() {
    if let Err(error) = auto_load_env() {
        eprintln!("veil: cannot load operator configuration: {}", error);
        process::exit(1);
    }
    // If VEILKEY_LOCALVAULT_URL is set, ensure VEILKEY_API matches
    if let Ok(url) = env::var("VEILKEY_LOCALVAULT_URL") {
        env::set_var("VEILKEY_API", &url);
    }

    let cli_bin = find_bin("VEILKEY_CLI_BIN", "veilkey-cli");

    env::set_var("VEIL_PS1", "(veil) ");

    let args: Vec<String> = env::args().skip(1).collect();

    if args.is_empty() {
        let rc = shell_rcfile().unwrap_or_else(|error| {
            eprintln!("veil: cannot create shell configuration: {}", error);
            process::exit(1);
        });
        // Keep the owner-only rcfile alive until the protected shell exits.
        let status = process::Command::new(&cli_bin)
            .args(["wrap-pty", "bash", "--rcfile"])
            .arg(rc.path())
            .status();
        drop(rc);
        match status {
            Ok(status) => process::exit(status.code().unwrap_or(1)),
            Err(error) => {
                eprintln!("veil: cannot start protected shell: {}", error);
                process::exit(1);
            }
        }
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

                    let status = if let Some(script_path) = find_repo_install_script() {
                        process::Command::new("bash")
                            .arg(script_path)
                            .env("VEILKEY_CENTER_URL", &center_url)
                            .env("VEILKEY_PORT", port.to_string())
                            .status()
                    } else {
                        eprintln!("veil localvault: no local installer found");
                        eprintln!("  Clone the latest release tag and run its install script.");
                        process::exit(1);
                    }
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
