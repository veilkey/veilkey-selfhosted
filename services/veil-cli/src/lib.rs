pub mod api;
pub mod commands;
pub mod config;
pub mod detector;
pub mod logger;
pub mod output;
pub mod project_config;
pub mod proxy;
pub mod pty;
pub mod state;

use std::env;
use std::path::Path;
use std::process::{Command, Stdio};

/// Proxy-related env vars to clear before loading session config.
const PROXY_VARS: &[&str] = &[
    "VEILKEY_PROXY_URL",
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "ALL_PROXY",
    "http_proxy",
    "https_proxy",
    "all_proxy",
];

/// Env vars whose values should have wrapping quotes stripped.
const SANITIZE_VARS: &[&str] = &[
    "VEILKEY_PROXY_URL",
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "ALL_PROXY",
    "NO_PROXY",
    "VEILKEY_LOCALVAULT_URL",
    "VEILKEY_KEYCENTER_URL",
    "VEILKEY_API",
    "VEILKEY_PLAINTEXT_ACTION",
];

pub fn clear_proxy_overrides() {
    for var in PROXY_VARS {
        env::remove_var(var);
    }
}

/// Strip one layer of matching surrounding quotes (single or double), repeatedly.
pub fn trim_wrapping_quotes(s: &str) -> String {
    let mut s = s.to_string();
    loop {
        let len = s.len();
        if len >= 2
            && ((s.starts_with('"') && s.ends_with('"'))
                || (s.starts_with('\'') && s.ends_with('\'')))
        {
            s = s[1..len - 1].to_string();
        } else {
            break;
        }
    }
    s
}

pub fn sanitize_exported_env() {
    for key in SANITIZE_VARS {
        if let Ok(val) = env::var(key) {
            if !val.is_empty() {
                env::set_var(key, trim_wrapping_quotes(&val));
            }
        }
    }
}

/// Parse a shell `export KEY='VALUE'` or `export KEY=VALUE` line.
/// Returns `None` for blank lines or lines without `=`.
pub fn parse_shell_export(line: &str) -> Option<(String, String)> {
    let line = line.trim();
    let line = line.strip_prefix("export ").unwrap_or(line);
    let eq = line.find('=')?;
    let key = &line[..eq];
    if key.is_empty() {
        return None;
    }
    let val = trim_wrapping_quotes(&line[eq + 1..]);
    Some((key.to_string(), val))
}

/// Run `session_config_bin shell-exports`, parse the output, and apply to the
/// current process environment.
pub fn load_session_exports(session_config_bin: &str) {
    let output = Command::new(session_config_bin)
        .arg("shell-exports")
        .stderr(Stdio::inherit())
        .output()
        .unwrap_or_else(|e| {
            eprintln!("failed to run {}: {}", session_config_bin, e);
            std::process::exit(1);
        });

    if !output.status.success() {
        eprintln!("{}: shell-exports failed", session_config_bin);
        std::process::exit(1);
    }

    for line in String::from_utf8_lossy(&output.stdout).lines() {
        if let Some((key, val)) = parse_shell_export(line) {
            env::set_var(&key, &val);
        }
    }
}

/// Check that `path` exists and is executable; exit with an error otherwise.
pub fn check_executable(path: &str, prog: &str) {
    use std::os::unix::fs::PermissionsExt;
    let p = Path::new(path);
    if !p.exists() {
        eprintln!("{}: required binary not found: {}", prog, path);
        std::process::exit(1);
    }
    match std::fs::metadata(p) {
        Ok(m) => {
            if m.permissions().mode() & 0o111 == 0 {
                eprintln!("{}: binary not executable: {}", prog, path);
                std::process::exit(1);
            }
        }
        Err(e) => {
            eprintln!("{}: cannot stat {}: {}", prog, path, e);
            std::process::exit(1);
        }
    }
}

/// Replace the current process image with `bin args` (Unix exec).
pub fn exec_replace(bin: &str, args: &[String]) -> ! {
    use std::os::unix::process::CommandExt;
    let err = Command::new(bin).args(args).exec();
    eprintln!("exec {}: {}", bin, err);
    std::process::exit(1);
}

/// Replace the current process image, setting argv[0] to `name`.
pub fn exec_as(name: &str, bin: &str, args: &[String]) -> ! {
    use std::os::unix::process::CommandExt;
    let err = Command::new(bin).arg0(name).args(args).exec();
    eprintln!("exec {}: {}", bin, err);
    std::process::exit(1);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_trim_wrapping_quotes_double() {
        assert_eq!(trim_wrapping_quotes("\"hello\""), "hello");
    }

    #[test]
    fn test_trim_wrapping_quotes_single() {
        assert_eq!(trim_wrapping_quotes("'hello'"), "hello");
    }

    #[test]
    fn test_trim_wrapping_quotes_nested() {
        // bash strips double-quotes first, then single-quotes — same here
        assert_eq!(trim_wrapping_quotes("\"'hello'\""), "hello");
    }

    #[test]
    fn test_trim_wrapping_quotes_none() {
        assert_eq!(trim_wrapping_quotes("hello"), "hello");
    }

    #[test]
    fn test_parse_shell_export_single_quotes() {
        let (k, v) = parse_shell_export("export FOO='bar'").unwrap();
        assert_eq!(k, "FOO");
        assert_eq!(v, "bar");
    }

    #[test]
    fn test_parse_shell_export_no_quotes() {
        let (k, v) = parse_shell_export("export FOO=bar").unwrap();
        assert_eq!(k, "FOO");
        assert_eq!(v, "bar");
    }

    #[test]
    fn test_parse_shell_export_url() {
        let (k, v) =
            parse_shell_export("export VEILKEY_LOCALVAULT_URL='http://127.0.0.1:5678'").unwrap();
        assert_eq!(k, "VEILKEY_LOCALVAULT_URL");
        assert_eq!(v, "http://127.0.0.1:5678");
    }

    #[test]
    fn test_parse_shell_export_empty_line() {
        assert!(parse_shell_export("").is_none());
        assert!(parse_shell_export("   ").is_none());
    }
}
