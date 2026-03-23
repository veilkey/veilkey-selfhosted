use std::sync::{Arc, RwLock};
use std::time::Duration;

use crate::api::{enrich_mask_map, VeilKeyClient};

/// Spawn a background thread that long-polls /api/mask-map for changes.
/// Updates the shared mask_map when secrets are added/changed/deleted.
pub fn spawn_mask_map_sync(
    mask_map: Arc<RwLock<Vec<(String, String)>>>,
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
                        if let Some(entries) = data["entries"].as_array() {
                            let mut new_map: Vec<(String, String)> = Vec::new();
                            for e in entries {
                                let r = e["ref"].as_str().unwrap_or_default();
                                let v = e["value"].as_str().unwrap_or_default();
                                let trimmed = v.trim_end_matches(['\r', '\n']);
                                if !trimmed.is_empty() && !r.is_empty() {
                                    new_map.push((trimmed.to_string(), r.to_string()));
                                }
                            }

                            enrich_mask_map(&mut new_map);

                            if let Ok(mut map) = mask_map.write() {
                                *map = new_map;
                                eprintln!(
                                    "[veilkey] mask_map synced: {} secret(s) (v{})",
                                    map.len(),
                                    new_version
                                );
                            }
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
