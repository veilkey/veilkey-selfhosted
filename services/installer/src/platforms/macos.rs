use clap::Args;
use colored::Colorize;
use std::process::Command;

#[derive(Args)]
pub struct MacosArgs {
    /// VaultCenter host port
    #[arg(long, default_value = "11181")]
    pub vaultcenter_port: u16,

    /// LocalVault host port
    #[arg(long, default_value = "11180")]
    pub localvault_port: u16,

    /// Skip Docker Compose (CLI only)
    #[arg(long)]
    pub cli_only: bool,
}

pub fn run(args: MacosArgs) -> Result<(), String> {
    println!("{}", "=== VeilKey Installer (macOS) ===".blue().bold());
    println!();

    // Check prerequisites
    check_cmd(
        "docker",
        "Install Docker Desktop: https://docs.docker.com/desktop/install/mac-install/",
    )?;
    check_cmd("npm", "Install Node.js: brew install node")?;
    check_cmd(
        "cargo",
        "Install Rust: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh",
    )?;
    println!("{}", "[1/4] Prerequisites OK".green());

    if !args.cli_only {
        // Start Docker Compose
        println!("[2/4] Starting services...");
        run_cmd("docker", &["compose", "up", "--build", "-d"])?;
        println!("{}", "  Services started".green());
    } else {
        println!("[2/4] Skipped (--cli-only)");
    }

    // Build CLI
    println!("[3/4] Building veil CLI...");
    run_cmd("cargo", &["build", "--release", "--quiet"])?;
    println!("{}", "  Built".green());

    // Install via npm + codesign
    println!("[4/4] Installing CLI...");
    run_cmd("bash", &["install/macos/veil-cli/install.sh"])?;
    println!("{}", "  Installed".green());

    println!();
    println!("{}", "=== Installation complete ===".green().bold());
    println!();
    println!("  VaultCenter: https://localhost:{}", args.vaultcenter_port);
    println!("  1. Open URL → set master + admin password");
    println!("  2. veil → enter protected shell");
    println!();

    Ok(())
}

fn check_cmd(name: &str, help: &str) -> Result<(), String> {
    if Command::new("which")
        .arg(name)
        .output()
        .map(|o| o.status.success())
        .unwrap_or(false)
    {
        Ok(())
    } else {
        Err(format!("{} not found.\n  {}", name, help))
    }
}

fn run_cmd(cmd: &str, args: &[&str]) -> Result<(), String> {
    let status = Command::new(cmd)
        .args(args)
        .status()
        .map_err(|e| format!("Failed to run {}: {}", cmd, e))?;
    if status.success() {
        Ok(())
    } else {
        Err(format!("{} exited with {}", cmd, status))
    }
}
