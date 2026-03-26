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
  veilkey create [value]            Create a temp ref (VK:TEMP:xxx)
  veilkey resolve <VK:ref>          Decrypt and print a VeilKey reference
  veilkey secret add <name> [val]   Create + promote to vault (one step)
  veilkey secret list [--vault url] List secrets in vault
  veilkey secret get <name>         Resolve secret value by name
  veilkey secret delete <name>      Delete secret from vault
  veilkey function list             List all global functions
  veilkey function add <name>       Create a global function
  veilkey function remove <name>    Delete a global function
  veilkey list                      List detected VeilKey entries
  veilkey paste-mode [mode]         Get or set pasted temp issuance mode
  veilkey clear                     Clear session log
  veilkey health [--json]            Show agent health from VaultCenter
  veilkey config search <key>        Search config across all vaults
  veilkey config bulk-update <k> <old> <new>  Update matching config values
  veilkey config bulk-set <key> <val>  Set config on all agents
  veilkey traefik <sub>              Manage Traefik config (init|status|destroy)
  veilkey status                    Show status
  veilkey version                   Show version

Options:
  --format <text|json|sarif>   Output format (default: text)
  --config <path>              Config file path (default: .veilkey.yml)
  --exit-code                  Exit with 1 if secrets found (for CI)
  --patterns <path>            Custom patterns file

Environment:
  VEILKEY_LOCALVAULT_URL       Preferred localvault URL
  VEILKEY_VAULTCENTER_URL      VaultCenter URL (for health/config commands)
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
        "config",
        "health",
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
        "exec" => commands::cmd_exec(&cmd_args, &api_url, &log_path, patterns_file.as_deref()),
        "list" => commands::cmd_list(&log_path),
        "paste-mode" => commands::cmd_paste_mode(&cmd_args),
        "clear" => commands::cmd_clear(&log_path),
        "status" => commands::cmd_status(&api_url, &log_path, patterns_file.as_deref(), VERSION),
        "resolve" => {
            // Must be interactive TTY — block pipe usage for security
            if unsafe { libc::isatty(0) } == 0 {
                eprintln!("[veilkey] resolve requires an interactive terminal (blocked in pipes)");
                process::exit(1);
            }
            let vk_ref = cmd_args.first().map(String::as_str).unwrap_or_else(|| {
                eprintln!("Usage: veilkey resolve <VK:ref>");
                process::exit(1);
            });
            let password = rpassword::prompt_password("VeilKey password: ").unwrap_or_default();
            let client = veil_cli_rs::api::VeilKeyClient::new(&api_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match client.resolve(vk_ref) {
                Ok(value) => println!("{}", value),
                Err(e) => {
                    eprintln!("[veilkey] resolve failed: {}", e);
                    process::exit(1);
                }
            }
        }
        "function" => {
            let subcmd = cmd_args.first().map(String::as_str).unwrap_or_else(|| {
                eprintln!("Usage: veilkey function <list|add|remove> [name]");
                process::exit(1);
            });
            let password = std::env::var("VEILKEY_PASSWORD").unwrap_or_else(|_| {
                rpassword::prompt_password("VeilKey password: ").unwrap_or_default()
            });
            let client = veil_cli_rs::api::VeilKeyClient::new(&api_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match subcmd {
                "list" => match client.function_list() {
                    Ok(functions) => {
                        if functions.is_empty() {
                            println!("No functions defined");
                        } else {
                            for f in &functions {
                                let name = f["name"].as_str().unwrap_or("?");
                                let category = f["category"].as_str().unwrap_or("");
                                if category.is_empty() {
                                    println!("{}", name);
                                } else {
                                    println!("{}  ({})", name, category);
                                }
                            }
                            println!("\nTotal: {} function(s)", functions.len());
                        }
                    }
                    Err(e) => {
                        eprintln!("[veilkey] function list failed: {}", e);
                        process::exit(1);
                    }
                },
                "add" => {
                    let name = cmd_args.get(1).map(String::as_str).unwrap_or_else(|| {
                        eprintln!("Usage: veilkey function add <name> [command]");
                        process::exit(1);
                    });
                    let command = cmd_args.get(2).map(String::as_str).unwrap_or(name);
                    match client.function_add(name, command) {
                        Ok(()) => println!("Function '{}' created", name),
                        Err(e) => {
                            eprintln!("[veilkey] function add failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "remove" => {
                    let name = cmd_args.get(1).map(String::as_str).unwrap_or_else(|| {
                        eprintln!("Usage: veilkey function remove <name>");
                        process::exit(1);
                    });
                    match client.function_remove(name) {
                        Ok(()) => println!("Function '{}' deleted", name),
                        Err(e) => {
                            eprintln!("[veilkey] function remove failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                unknown => {
                    eprintln!("Unknown function subcommand: {}", unknown);
                    eprintln!("Usage: veilkey function <list|add|remove> [name]");
                    process::exit(1);
                }
            }
        }
        "secret" => {
            let subcmd = cmd_args.first().map(String::as_str).unwrap_or_else(|| {
                eprintln!("Usage: veilkey secret <add|list|get|delete> [args]");
                process::exit(1);
            });
            let password = std::env::var("VEILKEY_PASSWORD").unwrap_or_else(|_| {
                rpassword::prompt_password("VeilKey password: ").unwrap_or_default()
            });
            let client = veil_cli_rs::api::VeilKeyClient::new(&api_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match subcmd {
                "add" => {
                    // veilkey secret add <name> [value] [--vault label]
                    let sub_args = &cmd_args[1..];
                    if sub_args.is_empty() {
                        eprintln!("Usage: veilkey secret add <name> [value] [--vault <label>]");
                        process::exit(1);
                    }
                    let name = &sub_args[0];

                    // Parse --vault flag
                    let mut vault_label: Option<String> = None;
                    let mut value_parts: Vec<&str> = Vec::new();
                    let mut i = 1;
                    while i < sub_args.len() {
                        if sub_args[i] == "--vault" && i + 1 < sub_args.len() {
                            vault_label = Some(sub_args[i + 1].clone());
                            i += 2;
                        } else {
                            value_parts.push(&sub_args[i]);
                            i += 1;
                        }
                    }

                    let value = if !value_parts.is_empty() {
                        value_parts.join(" ")
                    } else {
                        eprint!("Secret value: ");
                        rpassword::read_password().unwrap_or_else(|e| {
                            eprintln!("Failed to read input: {}", e);
                            process::exit(1);
                        })
                    };
                    let value = value.trim().to_string();
                    if value.is_empty() {
                        eprintln!("Value cannot be empty");
                        process::exit(1);
                    }

                    match client.secret_add(name, &value, vault_label.as_deref()) {
                        Ok(result) => {
                            let token = result["token"].as_str().unwrap_or("?");
                            let action = result["action"].as_str().unwrap_or("created");
                            println!("[veilkey] {} {} → {}", action, name, token);
                        }
                        Err(e) => {
                            eprintln!("[veilkey] secret add failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "list" => {
                    // veilkey secret list [--vault url]
                    let sub_args = &cmd_args[1..];
                    let lv_url = if sub_args.len() >= 2 && sub_args[0] == "--vault" {
                        sub_args[1].clone()
                    } else {
                        // Default: get agents list and pick the main vault's URL
                        match client.agents_list() {
                            Ok(agents) => {
                                let active: Vec<&serde_json::Value> = agents
                                    .iter()
                                    .filter(|a| !a["archived"].as_bool().unwrap_or(false))
                                    .collect();
                                if active.is_empty() {
                                    eprintln!("[veilkey] no active vaults found");
                                    process::exit(1);
                                }
                                // Print secrets for each vault
                                for agent in &active {
                                    let label = agent["label"].as_str().unwrap_or("unknown");
                                    let ip = agent["ip"].as_str().unwrap_or("?");
                                    let port = agent["port"].as_u64().unwrap_or(10180);
                                    let url = format!("https://{}:{}", ip, port);
                                    let count = agent["secrets_count"].as_u64().unwrap_or(0);
                                    println!("[{}] ({} secrets)", label, count);
                                    match client.secret_list(&url) {
                                        Ok(secrets) => {
                                            for s in &secrets {
                                                let name = s["name"].as_str().unwrap_or("?");
                                                let token = s["token"].as_str().unwrap_or("?");
                                                let scope = s["scope"].as_str().unwrap_or("?");
                                                println!("  {} {} ({})", token, name, scope);
                                            }
                                        }
                                        Err(e) => eprintln!("  error: {}", e),
                                    }
                                }
                                return;
                            }
                            Err(e) => {
                                eprintln!("[veilkey] agents list failed: {}", e);
                                process::exit(1);
                            }
                        }
                    };
                    match client.secret_list(&lv_url) {
                        Ok(secrets) => {
                            for s in &secrets {
                                let name = s["name"].as_str().unwrap_or("?");
                                let token = s["token"].as_str().unwrap_or("?");
                                let scope = s["scope"].as_str().unwrap_or("?");
                                println!("{} {} ({})", token, name, scope);
                            }
                        }
                        Err(e) => {
                            eprintln!("[veilkey] secret list failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "get" => {
                    // veilkey secret get <name> — resolve by name
                    if cmd_args.len() < 2 {
                        eprintln!("Usage: veilkey secret get <name>");
                        process::exit(1);
                    }
                    let name = &cmd_args[1];
                    // Find the ref for this name from agents/secrets
                    match client.agents_list() {
                        Ok(agents) => {
                            for agent in &agents {
                                if agent["archived"].as_bool().unwrap_or(false) {
                                    continue;
                                }
                                let ip = agent["ip"].as_str().unwrap_or("?");
                                let port = agent["port"].as_u64().unwrap_or(10180);
                                let url = format!("https://{}:{}", ip, port);
                                if let Ok(secrets) = client.secret_list(&url) {
                                    for s in &secrets {
                                        if s["name"].as_str() == Some(name.as_str()) {
                                            let token = s["token"].as_str().unwrap_or("?");
                                            match client.resolve(token) {
                                                Ok(value) => {
                                                    println!("{}", value);
                                                    return;
                                                }
                                                Err(e) => {
                                                    eprintln!("[veilkey] resolve failed: {}", e);
                                                    process::exit(1);
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                            eprintln!("[veilkey] secret '{}' not found", name);
                            process::exit(1);
                        }
                        Err(e) => {
                            eprintln!("[veilkey] agents list failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "delete" => {
                    // veilkey secret delete <name> [--vault url]
                    if cmd_args.len() < 2 {
                        eprintln!("Usage: veilkey secret delete <name> [--vault <url>]");
                        process::exit(1);
                    }
                    let name = &cmd_args[1];
                    let sub_args = &cmd_args[2..];
                    let lv_url = if sub_args.len() >= 2 && sub_args[0] == "--vault" {
                        sub_args[1].clone()
                    } else {
                        // Find vault containing the secret
                        match client.agents_list() {
                            Ok(agents) => {
                                let mut found_url = None;
                                for agent in &agents {
                                    if agent["archived"].as_bool().unwrap_or(false) {
                                        continue;
                                    }
                                    let ip = agent["ip"].as_str().unwrap_or("?");
                                    let port = agent["port"].as_u64().unwrap_or(10180);
                                    let url = format!("https://{}:{}", ip, port);
                                    if let Ok(secrets) = client.secret_list(&url) {
                                        if secrets
                                            .iter()
                                            .any(|s| s["name"].as_str() == Some(name.as_str()))
                                        {
                                            found_url = Some(url);
                                            break;
                                        }
                                    }
                                }
                                found_url.unwrap_or_else(|| {
                                    eprintln!("[veilkey] secret '{}' not found in any vault", name);
                                    process::exit(1);
                                })
                            }
                            Err(e) => {
                                eprintln!("[veilkey] agents list failed: {}", e);
                                process::exit(1);
                            }
                        }
                    };
                    match client.secret_delete(&lv_url, name) {
                        Ok(deleted) => println!("[veilkey] deleted: {}", deleted),
                        Err(e) => {
                            eprintln!("[veilkey] delete failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                unknown => {
                    eprintln!("Unknown secret subcommand: {}", unknown);
                    eprintln!("Usage: veilkey secret <add|list|get|delete> [args]");
                    process::exit(1);
                }
            }
        }
        "create" => {
            // veilkey create [value] — create a temp ref and print it
            let value = if !cmd_args.is_empty() {
                cmd_args.join(" ")
            } else {
                eprint!("Secret value: ");
                rpassword::read_password().unwrap_or_else(|e| {
                    eprintln!("Failed to read input: {}", e);
                    process::exit(1);
                })
            };
            let value = value.trim().to_string();
            if value.is_empty() {
                eprintln!("Value cannot be empty");
                process::exit(1);
            }
            let password = std::env::var("VEILKEY_PASSWORD").unwrap_or_else(|_| {
                rpassword::prompt_password("VeilKey password: ").unwrap_or_default()
            });
            let client = veil_cli_rs::api::VeilKeyClient::new(&api_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match client.issue(&value) {
                Ok(vk_ref) => println!("{}", vk_ref),
                Err(e) => {
                    eprintln!("[veilkey] create failed: {}", e);
                    process::exit(1);
                }
            }
        }
        "ssh" => {
            let subcmd = cmd_args.first().map(String::as_str).unwrap_or_else(|| {
                eprintln!("Usage: veilkey ssh <add|list|pubkey|remove> [args]");
                process::exit(1);
            });
            let password = std::env::var("VEILKEY_PASSWORD").unwrap_or_else(|_| {
                rpassword::prompt_password("VeilKey password: ").unwrap_or_default()
            });
            let client = veil_cli_rs::api::VeilKeyClient::new(&api_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match subcmd {
                "add" => {
                    let keyfile = cmd_args.get(1).map(String::as_str).unwrap_or_else(|| {
                        eprintln!("Usage: veilkey ssh add <keyfile> [--label name]");
                        process::exit(1);
                    });
                    let private_key = std::fs::read_to_string(keyfile).unwrap_or_else(|e| {
                        eprintln!("[veilkey] failed to read {}: {}", keyfile, e);
                        process::exit(1);
                    });
                    // Store private key as a secret via issue()
                    match client.issue_with_scope(private_key.trim(), "SSH") {
                        Ok(vk_ref) => {
                            println!("{}", vk_ref);

                            // Try to find and show public key
                            let pub_path = format!("{}.pub", keyfile);
                            if let Ok(pubkey) = std::fs::read_to_string(&pub_path) {
                                eprintln!("[veilkey] public key: {}", pubkey.trim());
                            }
                        }
                        Err(e) => {
                            eprintln!("[veilkey] ssh add failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "list" => {
                    // SSH keys are secrets with VK:SSH scope — list via mask_map, deduplicated
                    match client.fetch_all_secrets_mask_map() {
                        Some(map) => {
                            let mut seen = std::collections::HashSet::new();
                            let ssh_keys: Vec<_> = map
                                .iter()
                                .filter(|(_, r)| r.contains(":SSH:"))
                                .filter(|(_, r)| seen.insert(r.clone()))
                                .collect();
                            if ssh_keys.is_empty() {
                                println!("No SSH keys registered");
                            } else {
                                for (_, vk_ref) in &ssh_keys {
                                    println!("{}", vk_ref);
                                }
                                println!("\nTotal: {} key(s)", ssh_keys.len());
                            }
                        }
                        None => {
                            eprintln!("[veilkey] failed to fetch keys");
                            process::exit(1);
                        }
                    }
                }
                _ => {
                    eprintln!("Unknown ssh subcommand: {}", subcmd);
                    eprintln!("Usage: veilkey ssh <add|list> [args]");
                    process::exit(1);
                }
            }
        }
        "health" => {
            let vc_url = std::env::var("VEILKEY_VAULTCENTER_URL").unwrap_or_else(|_| {
                eprintln!("ERROR: VEILKEY_VAULTCENTER_URL is required for health command.");
                eprintln!("  export VEILKEY_VAULTCENTER_URL=<vaultcenter-url>");
                process::exit(1);
            });
            let json_flag = cmd_args.iter().any(|a| a == "--json");
            let password = std::env::var("VEILKEY_PASSWORD").unwrap_or_else(|_| {
                rpassword::prompt_password("VeilKey password: ").unwrap_or_default()
            });
            let client = veil_cli_rs::api::VeilKeyClient::new(&vc_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match client.agents_list() {
                Ok(agents) => {
                    if json_flag {
                        let out = serde_json::json!({ "agents": agents });
                        println!("{}", serde_json::to_string_pretty(&out).unwrap_or_default());
                    } else if agents.is_empty() {
                        println!("No agents registered");
                    } else {
                        println!(
                            "{:<20} {:<10} {:>8} {}",
                            "AGENT", "STATUS", "SECRETS", "LAST_SEEN"
                        );
                        println!("{}", "-".repeat(60));
                        for agent in &agents {
                            let label = agent["label"].as_str().unwrap_or("?");
                            let archived = agent["archived"].as_bool().unwrap_or(false);
                            let status = if archived {
                                "archived"
                            } else {
                                agent["status"].as_str().unwrap_or("unknown")
                            };
                            let secrets = agent["secrets_count"].as_u64().unwrap_or(0);
                            let last_seen = agent["last_seen_ago"]
                                .as_str()
                                .or_else(|| agent["last_heartbeat"].as_str())
                                .unwrap_or("-");
                            println!("{:<20} {:<10} {:>8} {}", label, status, secrets, last_seen);
                        }
                        println!("\nTotal: {} agent(s)", agents.len());
                    }
                }
                Err(e) => {
                    eprintln!("[veilkey] health check failed: {}", e);
                    process::exit(1);
                }
            }
        }
        "config" => {
            let vc_url = std::env::var("VEILKEY_VAULTCENTER_URL").unwrap_or_else(|_| {
                eprintln!("ERROR: VEILKEY_VAULTCENTER_URL is required for config commands.");
                eprintln!("  export VEILKEY_VAULTCENTER_URL=<vaultcenter-url>");
                process::exit(1);
            });
            let subcmd = cmd_args.first().map(String::as_str).unwrap_or_else(|| {
                eprintln!("Usage: veilkey config <search|bulk-update|bulk-set> [args]");
                process::exit(1);
            });
            let password = std::env::var("VEILKEY_PASSWORD").unwrap_or_else(|_| {
                rpassword::prompt_password("VeilKey password: ").unwrap_or_default()
            });
            let client = veil_cli_rs::api::VeilKeyClient::new(&vc_url);
            if let Err(e) = client.admin_login(&password) {
                eprintln!("[veilkey] login failed: {}", e);
                process::exit(1);
            }
            match subcmd {
                "search" => {
                    let key = cmd_args.get(1).map(String::as_str).unwrap_or_else(|| {
                        eprintln!("Usage: veilkey config search <key>");
                        process::exit(1);
                    });
                    match client.configs_search(key) {
                        Ok(result) => {
                            if let Some(results) = result["results"].as_array() {
                                if results.is_empty() {
                                    println!("No configs found for '{}'", key);
                                } else {
                                    for entry in results {
                                        let label = entry["label"].as_str().unwrap_or("?");
                                        let value = entry["value"].as_str().unwrap_or("?");
                                        println!("  {} = {}", label, value);
                                    }
                                    println!("\nTotal: {} agent(s)", results.len());
                                }
                            } else if let Some(entries) = result["entries"].as_array() {
                                if entries.is_empty() {
                                    println!("No configs found for '{}'", key);
                                } else {
                                    for entry in entries {
                                        let agent = entry["agent"]
                                            .as_str()
                                            .or_else(|| entry["label"].as_str())
                                            .unwrap_or("?");
                                        let value = entry["value"].as_str().unwrap_or("?");
                                        println!("  {} = {}", agent, value);
                                    }
                                    println!("\nTotal: {} agent(s)", entries.len());
                                }
                            } else {
                                println!(
                                    "{}",
                                    serde_json::to_string_pretty(&result).unwrap_or_default()
                                );
                            }
                        }
                        Err(e) => {
                            eprintln!("[veilkey] config search failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "bulk-update" => {
                    if cmd_args.len() < 4 {
                        eprintln!(
                            "Usage: veilkey config bulk-update <key> <old_value> <new_value>"
                        );
                        process::exit(1);
                    }
                    let key = &cmd_args[1];
                    let old_value = &cmd_args[2];
                    let new_value = &cmd_args[3];
                    match client.configs_bulk_update(key, old_value, new_value) {
                        Ok(result) => {
                            let updated = result["updated"].as_u64().unwrap_or(0);
                            let total = result["total"].as_u64().unwrap_or(0);
                            println!(
                                "[veilkey] bulk-update: {}/{} agents updated",
                                updated, total
                            );
                            if let Some(errors) = result["errors"].as_array() {
                                for err in errors {
                                    let agent = err["agent"].as_str().unwrap_or("?");
                                    let msg = err["error"].as_str().unwrap_or("?");
                                    eprintln!("  ERROR {}: {}", agent, msg);
                                }
                            }
                        }
                        Err(e) => {
                            eprintln!("[veilkey] bulk-update failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                "bulk-set" => {
                    if cmd_args.len() < 3 {
                        eprintln!("Usage: veilkey config bulk-set <key> <value>");
                        process::exit(1);
                    }
                    let key = &cmd_args[1];
                    let value = &cmd_args[2];
                    match client.configs_bulk_set(key, value) {
                        Ok(result) => {
                            let updated = result["updated"].as_u64().unwrap_or(0);
                            let total = result["total"].as_u64().unwrap_or(0);
                            println!("[veilkey] bulk-set: {}/{} agents updated", updated, total);
                            if let Some(errors) = result["errors"].as_array() {
                                for err in errors {
                                    let agent = err["agent"].as_str().unwrap_or("?");
                                    let msg = err["error"].as_str().unwrap_or("?");
                                    eprintln!("  ERROR {}: {}", agent, msg);
                                }
                            }
                        }
                        Err(e) => {
                            eprintln!("[veilkey] bulk-set failed: {}", e);
                            process::exit(1);
                        }
                    }
                }
                unknown => {
                    eprintln!("Unknown config subcommand: {}", unknown);
                    eprintln!("Usage: veilkey config <search|bulk-update|bulk-set> [args]");
                    process::exit(1);
                }
            }
        }
        "version" => println!("veilkey {}", VERSION),
        "traefik" => {
            commands::cmd_traefik(&cmd_args, &api_url, &log_path, patterns_file.as_deref())
        }
        "help" | "-h" | "--help" => print_usage(),
        unknown => {
            eprintln!("Unknown command: {}", unknown);
            print_usage();
            process::exit(1);
        }
    }
}
