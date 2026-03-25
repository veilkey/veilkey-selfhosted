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
                    if changed && new_version >= version {
                        let (mut new_map, new_ve) = parse_mask_map_entries(&data);

                        enrich_mask_map(&mut new_map);

                        if let Ok(mut map) = mask_map.write() {
                            *map = new_map;
                            eprintln!(
                                "[veilkey] mask_map synced: {} secret(s), {} config(s) (v{})",
                                map.len(),
                                new_ve.len(),
                                new_version
                            );
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
