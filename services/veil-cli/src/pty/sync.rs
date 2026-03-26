use std::sync::{Arc, RwLock};
use std::time::Duration;

use crate::api::{enrich_mask_map, parse_mask_map_entries, VeilKeyClient};

/// Spawn a background thread that long-polls /api/mask-map for changes.
/// Updates the shared mask_map (and ve_map) when secrets are added/changed/deleted.
pub fn spawn_mask_map_sync(
    mask_map: Arc<RwLock<Vec<(String, String)>>>,
    ve_map: Arc<RwLock<Vec<(String, String)>>>,
    client: Arc<VeilKeyClient>,
) {
    std::thread::spawn(move || {
        let mut version: u64 = 0;
        // Long poll timeout must exceed server wait (30s) + margin
        let poll_timeout = Duration::from_secs(45);
        loop {
            let url = format!(
                "{}/api/mask-map?version={}&wait=30",
                client.base_url(),
                version
            );
            match client.raw_get_with_timeout(&url, poll_timeout) {
                Ok(resp) => {
                    let data: serde_json::Value = resp.into_json().unwrap_or_default();
                    let new_version = data["version"].as_u64().unwrap_or(version);
                    let changed = data["changed"].as_bool().unwrap_or(false);
                    if changed && new_version > version {
                        let (mut new_map, new_ve) = parse_mask_map_entries(&data);

                        enrich_mask_map(&mut new_map);

                        if let Ok(mut map) = mask_map.write() {
                            let _old_len = map.len();
                            *map = new_map;
                            // Silently update — no terminal output during active shell
                        }
                        if let Ok(mut ve) = ve_map.write() {
                            *ve = new_ve;
                        }
                        version = new_version;
                    } else {
                        version = new_version;
                    }
                }
                Err(_) => {
                    std::thread::sleep(Duration::from_secs(10));
                }
            }
        }
    });
}

#[cfg(test)]
mod sync_tests {
    use std::fs;

    /// SECURITY/UX: sync message must only print when version INCREASES (>), not >=.
    /// If >= is used, every long-poll response triggers a message (spam).
    #[test]
    fn sync_uses_strict_version_comparison() {
        let src = fs::read_to_string("src/pty/sync.rs").expect("read sync.rs");
        // Only check non-test code (before #[cfg(test)])
        let prod = src.split("#[cfg(test)]").next().unwrap_or(&src);
        assert!(
            prod.contains("new_version > version"),
            "sync must use strict > comparison to prevent spam"
        );
        // >= would cause spam on every long-poll
        let ge = format!("new_version {} version", ">=");
        assert!(
            !prod.contains(&ge),
            "SPAM BUG: >= causes message on every poll response"
        );
    }

    /// Sync message must only appear when count changes, not on every update.
    #[test]
    fn sync_no_terminal_output() {
        // Sync must not print to terminal (disrupts prompt layout)
        let src = fs::read_to_string("src/pty/sync.rs").expect("read sync.rs");
        let prod = src.split("#[cfg(test)]").next().unwrap_or(&src);
        assert!(
            !prod.contains("eprintln!"),
            "sync must not use eprintln! (disrupts active terminal)"
        );
    }

    /// Version must be tracked and updated after successful sync.
    #[test]
    fn sync_updates_version_after_change() {
        let src = fs::read_to_string("src/pty/sync.rs").expect("read sync.rs");
        assert!(
            src.contains("version = new_version"),
            "version must be updated after successful sync"
        );
    }
}
