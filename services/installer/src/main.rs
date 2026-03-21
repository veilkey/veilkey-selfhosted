mod platforms;

use clap::{Parser, Subcommand};
use colored::Colorize;

#[derive(Parser)]
#[command(name = "veilkey-installer")]
#[command(about = "Cross-platform installer for VeilKey Self-Hosted")]
#[command(version)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Detect platform and show recommended install command
    Detect,
    /// Install on macOS (Docker + CLI)
    Macos(platforms::macos::MacosArgs),
    /// Install on Proxmox LXC with Debian
    ProxmoxLxcDebian(platforms::proxmox::ProxmoxArgs),
    /// Install veil-cli on any Linux
    VeilCli(platforms::veil_cli::VeilCliArgs),
}

fn main() {
    let cli = Cli::parse();

    let result = match cli.command {
        Commands::Detect => platforms::detect::run(),
        Commands::Macos(args) => platforms::macos::run(args),
        Commands::ProxmoxLxcDebian(args) => platforms::proxmox::run(args),
        Commands::VeilCli(args) => platforms::veil_cli::run(args),
    };

    if let Err(e) = result {
        eprintln!("{} {}", "ERROR:".red().bold(), e);
        std::process::exit(1);
    }
}
