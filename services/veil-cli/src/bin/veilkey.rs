use std::env;
use veil_cli_rs::{
    check_executable, clear_proxy_overrides, exec_as, exec_replace, load_session_exports,
    sanitize_exported_env,
};

fn load_session_env(session_config_bin: &str) {
    check_executable(session_config_bin, "veilkey");
    clear_proxy_overrides();
    load_session_exports(session_config_bin);
    sanitize_exported_env();
}

fn print_usage() {
    println!("Usage:");
    println!("  veilkey session [command...]         Start a protected session or run a command via wrap-pty");
    println!("  veilkey status                       Show VeilKey status");
    println!("  veilkey paste-mode [on|off|status]   Control standalone pasted temp issuance");
    println!("  veilkey encrypt                      Encrypt plaintext into a VeilKey ref");
    println!("  veilkey scan [args...]               Run secret scan");
    println!("  veilkey filter [args...]             Run secret filter");
    println!("  veilkey exec [args...]               Resolve refs and execute command");
    println!("  veilkey proxy [args...]              Run local egress proxy");
    println!("  veilkey help                         Show this help");
}

fn main() {
    let veilkey_cli_bin = env::var("VEILKEY_CLI_BIN").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_CLI_BIN is required");
        std::process::exit(1);
    });
    let vk_bin = env::var("VEILKEY_VK_BIN").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_VK_BIN is required");
        std::process::exit(1);
    });
    let session_config_bin = env::var("VEILKEY_SESSION_CONFIG_BIN").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_SESSION_CONFIG_BIN is required");
        std::process::exit(1);
    });

    check_executable(&veilkey_cli_bin, "veilkey");

    let args: Vec<String> = env::args().skip(1).collect();
    let cmd = args.first().map(|s| s.as_str()).unwrap_or("help");

    match cmd {
        "help" | "-h" | "--help" => {
            print_usage();
        }
        "session" => {
            // exec -a veil veilkey-cli wrap-pty [args...]
            let mut full = vec!["wrap-pty".to_string()];
            full.extend_from_slice(args.get(1..).unwrap_or(&[]));
            exec_as("veil", &veilkey_cli_bin, &full);
        }
        "status" | "paste-mode" | "scan" | "filter" | "exec" | "resolve" | "proxy" | "list"
        | "clear" | "function" | "version" => {
            load_session_env(&session_config_bin);
            let mut full = vec![cmd.to_string()];
            full.extend_from_slice(args.get(1..).unwrap_or(&[]));
            exec_replace(&veilkey_cli_bin, &full);
        }
        "encrypt" => {
            check_executable(&vk_bin, "veilkey");
            exec_replace(&vk_bin, args.get(1..).unwrap_or(&[]));
        }
        "wrap" | "wrap-pty" => {
            load_session_env(&session_config_bin);
            let mut full = vec![cmd.to_string()];
            full.extend_from_slice(args.get(1..).unwrap_or(&[]));
            exec_as("veil", &veilkey_cli_bin, &full);
        }
        _ => {
            // Pass through: veilkey-cli <cmd> [args from $2 onwards]
            load_session_env(&session_config_bin);
            let mut full = vec![cmd.to_string()];
            full.extend_from_slice(args.get(1..).unwrap_or(&[]));
            exec_replace(&veilkey_cli_bin, &full);
        }
    }
}
