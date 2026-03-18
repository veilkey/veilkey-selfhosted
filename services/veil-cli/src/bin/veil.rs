use std::env;
use veil_cli_rs::{
    check_executable, clear_proxy_overrides, exec_replace, load_session_exports,
    sanitize_exported_env,
};

fn main() {
    let session_config_bin = env::var("VEILKEY_SESSION_CONFIG_BIN").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_SESSION_CONFIG_BIN is required");
        std::process::exit(1);
    });
    let veilkey_bin = env::var("VEILKEY_BIN").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_BIN is required");
        std::process::exit(1);
    });

    check_executable(&session_config_bin, "veil");
    check_executable(&veilkey_bin, "veil");

    clear_proxy_overrides();
    load_session_exports(&session_config_bin);
    sanitize_exported_env();

    env::set_var("VEILKEY_VEIL", "1");
    env::set_var("VEILKEY_VERIFIED_SESSION", "1");

    let args: Vec<String> = env::args().skip(1).collect();

    if args.is_empty() {
        exec_replace(
            &veilkey_bin,
            &[
                "session".to_string(),
                "bash".to_string(),
                "-li".to_string(),
            ],
        );
    }

    match args[0].as_str() {
        "status" => {
            let mut full = vec!["status".to_string()];
            full.extend_from_slice(&args[1..]);
            exec_replace(&veilkey_bin, &full);
        }
        "paste-mode" => {
            let mut full = vec!["paste-mode".to_string()];
            full.extend_from_slice(&args[1..]);
            exec_replace(&veilkey_bin, &full);
        }
        _ => {
            eprintln!("veil: direct app launch is not supported; enter veil shell first and run the tool inside it.");
            eprintln!("usage: veil [status|paste-mode [on|off|status]]");
            std::process::exit(1);
        }
    }
}
