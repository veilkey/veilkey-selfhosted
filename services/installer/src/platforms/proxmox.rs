use clap::Args;
use colored::Colorize;
use std::process::Command;

#[derive(Args)]
pub struct ProxmoxArgs {
    /// Container IP address (e.g. 10.50.0.110/16)
    #[arg(long)]
    pub ip: String,

    /// Gateway IP address
    #[arg(long)]
    pub gateway: String,

    /// Container ID (auto-detected if not set)
    #[arg(long)]
    pub ctid: Option<u32>,

    /// Container hostname
    #[arg(long, default_value = "veilkey")]
    pub hostname: String,

    /// Network bridge
    #[arg(long, default_value = "vmbr1")]
    pub bridge: String,

    /// Memory in MB
    #[arg(long, default_value = "2048")]
    pub memory: u32,

    /// CPU cores
    #[arg(long, default_value = "2")]
    pub cores: u32,

    /// Disk size in GB
    #[arg(long, default_value = "16")]
    pub disk: u32,

    /// Storage backend
    #[arg(long, default_value = "local-lvm")]
    pub storage: String,

    /// LXC template
    #[arg(long, default_value = "debian-13-standard_13.1-2_amd64.tar.zst")]
    pub template: String,
}

pub fn run(args: ProxmoxArgs) -> Result<(), String> {
    println!(
        "{}",
        "=== VeilKey Installer (Proxmox LXC Debian) ==="
            .blue()
            .bold()
    );
    println!();

    check_cmd("pct", "This command must run on a Proxmox host")?;

    let ctid = match args.ctid {
        Some(id) => id,
        None => {
            let output = Command::new("pvesh")
                .args(["get", "/cluster/nextid"])
                .output()
                .map_err(|e| format!("Failed to get next ID: {}", e))?;
            String::from_utf8_lossy(&output.stdout)
                .trim()
                .parse::<u32>()
                .map_err(|e| format!("Failed to parse CTID: {}", e))?
        }
    };

    println!("  CTID:     {}", ctid);
    println!("  Hostname: {}", args.hostname);
    println!("  IP:       {}", args.ip);
    println!("  Gateway:  {}", args.gateway);
    println!("  Bridge:   {}", args.bridge);
    println!("  Memory:   {}MB", args.memory);
    println!("  Cores:    {}", args.cores);
    println!("  Disk:     {}GB", args.disk);
    println!();

    // Delegate to bash script (proven and tested)
    let status = Command::new("bash")
        .args(["install/proxmox-lxc-debian/install-veilkey.sh"])
        .env("CTID", ctid.to_string())
        .env("CT_HOSTNAME", &args.hostname)
        .env("CT_IP", &args.ip)
        .env("CT_GW", &args.gateway)
        .env("CT_BRIDGE", &args.bridge)
        .env("CT_MEMORY", args.memory.to_string())
        .env("CT_CORES", args.cores.to_string())
        .env("CT_DISK", args.disk.to_string())
        .env("CT_STORAGE", &args.storage)
        .env("CT_TEMPLATE", &args.template)
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
