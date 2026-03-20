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

fn main() {
    let cli_bin = find_bin("VEILKEY_CLI_BIN", "veilkey-cli");

    env::set_var("VEILKEY_VEIL", "1");
    // Set custom prompt so user knows they're in veil
    env::set_var("VEIL_PS1", "(veil) ");

    let args: Vec<String> = env::args().skip(1).collect();

    if args.is_empty() {
        // Create temp rcfile for custom prompt
        let rc_path = format!("{}/veil-bashrc", env::temp_dir().display());
        let rc_content = r#"
[ -f ~/.bashrc ] && source ~/.bashrc
[ -f ~/.bash_profile ] && source ~/.bash_profile
export PS1="\[\033[36m\](VEIL)\[\033[0m\] \h:\W \u\$ "
"#;
        let _ = std::fs::write(&rc_path, rc_content);

        // veil → enter protected shell with custom prompt
        exec_replace(
            &cli_bin,
            &["wrap-pty".to_string(), "bash".to_string(), "--rcfile".to_string(), rc_path],
        );
    }

    match args[0].as_str() {
        "status" => {
            exec_replace(&cli_bin, &["status".to_string()]);
        }
        "resolve" => {
            let mut full = vec!["resolve".to_string()];
            full.extend_from_slice(&args[1..]);
            exec_replace(&cli_bin, &full);
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
        "help" | "-h" | "--help" => {
            println!("Usage:");
            println!("  veil                     Enter protected session (PTY masking)");
            println!("  veil status              Show VeilKey connection status");
            println!("  veil resolve <VK:ref>    Resolve a VK reference");
            println!("  veil exec <command...>   Resolve VK refs in args and execute");
            println!("  veil scan [file...]      Scan files for secrets");
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
