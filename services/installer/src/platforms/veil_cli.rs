use clap::Args;
use colored::Colorize;
use std::process::Command;

#[derive(Args)]
pub struct VeilCliArgs {
    /// VeilKey server URL (required)
    #[arg(long)]
    pub url: String,
}

pub fn run(args: VeilCliArgs) -> Result<(), String> {
    println!("{}", "=== VeilKey Installer (veil-cli) ===".blue().bold());
    println!();
    println!("  URL: {}", args.url);
    println!();

    check_cmd(
        "cargo",
        "Install Rust: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh",
    )?;
    println!("{}", "[1/1] Prerequisites OK".green());

    // Delegate to bash script
    let status = Command::new("bash")
        .args(["install/common/install-veil-cli.sh"])
        .env("VEILKEY_URL", &args.url)
        .status()
        .map_err(|e| format!("Failed to run installer: {}", e))?;

    if status.success() {
        Ok(())
    } else {
        Err(format!("Installer exited with {}", status))
    }
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
