use colored::Colorize;
use std::path::Path;

pub fn run() -> Result<(), String> {
    println!("{}", "=== VeilKey Platform Detection ===".blue().bold());
    println!();

    let os = std::env::consts::OS;
    let arch = std::env::consts::ARCH;
    println!("  OS:   {}", os);
    println!("  Arch: {}", arch);

    match os {
        "macos" => {
            println!("  Platform: {}", "macOS".green());
            println!();
            println!("Recommended:");
            println!("  veilkey-installer macos");
        }
        "linux" => {
            if Path::new("/usr/bin/pct").exists() || Path::new("/usr/sbin/pct").exists() {
                println!("  Platform: {}", "Proxmox VE".green());
                println!();
                println!("Recommended:");
                println!("  veilkey-installer proxmox-lxc-debian --ip <IP>/<MASK> --gateway <GW>");
            } else {
                println!("  Platform: {}", "Linux".green());
                println!();
                println!("Recommended:");
                println!("  veilkey-installer veil-cli --url <VEILKEY_URL>");
            }
        }
        _ => {
            println!("  Platform: {} (unsupported)", os.red());
            return Err(format!("Unsupported platform: {}", os));
        }
    }

    println!();
    Ok(())
}
