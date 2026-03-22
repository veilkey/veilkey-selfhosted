use base64::Engine as _;
use serde::Deserialize;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

fn build_agent() -> ureq::Agent {
    let insecure = std::env::var("VEILKEY_TLS_INSECURE").unwrap_or_default() == "1";
    let timeout = std::time::Duration::from_secs(10);
    if insecure {
        let config = rustls::ClientConfig::builder()
            .dangerous()
            .with_custom_certificate_verifier(Arc::new(NoVerifier))
            .with_no_client_auth();
        ureq::AgentBuilder::new()
            .tls_config(Arc::new(config))
            .timeout_connect(timeout)
            .timeout_read(timeout)
            .timeout_write(timeout)
            .build()
    } else {
        ureq::AgentBuilder::new()
            .timeout_connect(timeout)
            .timeout_read(timeout)
            .timeout_write(timeout)
            .build()
    }
}

#[derive(Debug)]
struct NoVerifier;
impl rustls::client::danger::ServerCertVerifier for NoVerifier {
    fn verify_server_cert(
        &self,
        _end_entity: &rustls::pki_types::CertificateDer<'_>,
        _intermediates: &[rustls::pki_types::CertificateDer<'_>],
        _server_name: &rustls::pki_types::ServerName<'_>,
        _ocsp_response: &[u8],
        _now: rustls::pki_types::UnixTime,
    ) -> Result<rustls::client::danger::ServerCertVerified, rustls::Error> {
        Ok(rustls::client::danger::ServerCertVerified::assertion())
    }
    fn verify_tls12_signature(
        &self,
        _: &[u8],
        _: &rustls::pki_types::CertificateDer<'_>,
        _: &rustls::DigitallySignedStruct,
    ) -> Result<rustls::client::danger::HandshakeSignatureValid, rustls::Error> {
        Ok(rustls::client::danger::HandshakeSignatureValid::assertion())
    }
    fn verify_tls13_signature(
        &self,
        _: &[u8],
        _: &rustls::pki_types::CertificateDer<'_>,
        _: &rustls::DigitallySignedStruct,
    ) -> Result<rustls::client::danger::HandshakeSignatureValid, rustls::Error> {
        Ok(rustls::client::danger::HandshakeSignatureValid::assertion())
    }
    fn supported_verify_schemes(&self) -> Vec<rustls::SignatureScheme> {
        rustls::crypto::ring::default_provider()
            .signature_verification_algorithms
            .supported_schemes()
    }
}

#[derive(Clone)]
pub struct VeilKeyClient {
    base_url: String,
    agent: ureq::Agent,
    cache: Arc<Mutex<HashMap<String, String>>>,
}

#[derive(Debug, Deserialize)]
#[allow(dead_code)]
pub struct ExactLookupMatch {
    pub r#ref: String,
    pub family: String,
    pub scope: String,
    pub id: String,
    pub secret_name: Option<String>,
    pub status: Option<String>,
}

impl VeilKeyClient {
    pub fn new(base_url: &str) -> Self {
        Self {
            base_url: base_url.trim_end_matches('/').to_string(),
            agent: build_agent(),
            cache: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    pub fn issue(&self, value: &str) -> Result<String, String> {
        let value = value.trim_end_matches(['\r', '\n']);
        {
            let cache = self.cache.lock().unwrap();
            if let Some(vk) = cache.get(value) {
                return Ok(vk.clone());
            }
        }

        let body = serde_json::json!({ "plaintext": value });
        let resp = self
            .agent
            .post(&format!("{}/api/encrypt", self.base_url))
            .set("Content-Type", "application/json")
            .send_json(&body)
            .map_err(|e| format!("API request failed: {}", e))?;

        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("API response decode failed: {}", e))?;

        let token = result["token"]
            .as_str()
            .ok_or("missing token in response")?
            .to_string();

        self.cache
            .lock()
            .unwrap()
            .insert(value.to_string(), token.clone());
        Ok(token)
    }

    pub fn resolve(&self, token: &str) -> Result<String, String> {
        let candidates = resolve_candidates(token);
        let mut last_err = String::from("resolve failed: no candidates");
        for candidate in &candidates {
            match self.resolve_once(candidate) {
                Ok(v) => return Ok(v),
                Err(e) => last_err = e,
            }
        }
        Err(last_err)
    }

    fn resolve_once(&self, r#ref: &str) -> Result<String, String> {
        let url = format!(
            "{}/api/resolve/{}",
            self.base_url,
            urlencoding::encode(r#ref)
        );
        let resp = self
            .agent
            .get(&url)
            .call()
            .map_err(|e| format!("resolve request failed: {}", e))?;

        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("resolve decode failed: {}", e))?;

        result["value"]
            .as_str()
            .map(|s| s.to_string())
            .ok_or_else(|| "missing value in response".to_string())
    }

    #[allow(dead_code)]
    pub fn exact_lookup(&self, plaintext: &str) -> Result<Vec<ExactLookupMatch>, String> {
        let body = serde_json::json!({ "plaintext": plaintext });
        let resp = self
            .agent
            .post(&format!("{}/api/lookup/exact", self.base_url))
            .set("Content-Type", "application/json")
            .send_json(&body)
            .map_err(|e| format!("lookup request failed: {}", e))?;

        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("lookup decode failed: {}", e))?;

        let matches = result["matches"]
            .as_array()
            .ok_or("missing matches in response")?;

        let mut out = Vec::new();
        for m in matches {
            if let Ok(item) = serde_json::from_value::<ExactLookupMatch>(m.clone()) {
                out.push(item);
            }
        }
        Ok(out)
    }

    /// Fetch all secrets from all vaults and resolve each to build a mask map.
    /// Returns Vec<(plaintext, vk_ref)> sorted by plaintext length descending.
    /// Returns None if the API is unreachable after retries (fail-closed).
    pub fn fetch_all_secrets_mask_map(&self) -> Option<Vec<(String, String)>> {
        let mut mask_map: Vec<(String, String)> = Vec::new();

        // 1. Get all tracked refs (no auth required, no values)
        //    Retry up to 3 times with backoff on failure (fail-closed: None on total failure)
        let max_retries: u32 = std::env::var("VEILKEY_MASK_FETCH_RETRIES")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(3);
        let mut ref_entries: Option<Vec<serde_json::Value>> = None;
        for attempt in 0..max_retries {
            if attempt > 0 {
                let delay = std::time::Duration::from_millis(500 * (1 << attempt));
                eprintln!(
                    "[veilkey] retrying refs fetch in {}ms (attempt {}/{})",
                    delay.as_millis(),
                    attempt + 1,
                    max_retries
                );
                std::thread::sleep(delay);
            }
            match self
                .agent
                .get(&format!("{}/api/refs", self.base_url))
                .call()
            {
                Ok(resp) => {
                    let data: serde_json::Value = resp.into_json().unwrap_or_default();
                    ref_entries = Some(data["refs"].as_array().cloned().unwrap_or_default());
                    break;
                }
                Err(e) => {
                    eprintln!(
                        "[veilkey] failed to fetch refs (attempt {}/{}): {}",
                        attempt + 1,
                        max_retries,
                        e
                    );
                }
            }
        }
        let ref_entries = match ref_entries {
            Some(entries) => entries,
            None => {
                eprintln!(
                    "[veilkey] FATAL: could not fetch refs after {} attempts — fail-closed",
                    max_retries
                );
                return None;
            }
        };

        // 2. Resolve each ref to get plaintext → canonical mapping
        for entry in &ref_entries {
            let canonical = match entry["ref_canonical"].as_str() {
                Some(c) => c,
                None => continue,
            };
            let resolve_url = format!(
                "{}/api/resolve/{}",
                self.base_url,
                urlencoding::encode(canonical)
            );
            let resolve_resp = self.agent.get(&resolve_url).call();
            if let Err(e) = &resolve_resp {
                eprintln!("[veilkey] resolve {} failed: {}", canonical, e);
                continue;
            }
            if let Ok(resp) = resolve_resp {
                let data: serde_json::Value = resp.into_json().unwrap_or_default();
                if let Some(value) = data["value"].as_str() {
                    let trimmed = value.trim_end_matches(['\r', '\n']);
                    if !trimmed.is_empty() {
                        mask_map.push((trimmed.to_string(), canonical.to_string()));
                    }
                }
            }
        }

        // Deduplicate: same plaintext → prefer VK:LOCAL over VK:TEMP
        let mut deduped: std::collections::HashMap<String, String> =
            std::collections::HashMap::new();
        for (plaintext, vk_ref) in &mask_map {
            let existing = deduped.get(plaintext);
            let prefer_new = match existing {
                None => true,
                Some(old) => old.contains(":TEMP:") && vk_ref.contains(":LOCAL:"),
            };
            if prefer_new {
                deduped.insert(plaintext.clone(), vk_ref.clone());
            }
        }
        let mut result: Vec<(String, String)> = deduped.into_iter().collect();

        // 4. Add encoded variants (base64, hex) so encoded secrets are also masked.
        //    Each variant maps back to the same VK ref as the original plaintext.
        //    Only add variants for secrets >= 8 chars to avoid false positives.
        let mut encoded_variants: Vec<(String, String)> = Vec::new();
        for (plaintext, vk_ref) in &result {
            if plaintext.len() < 8 {
                continue;
            }
            // base64 standard
            let b64_std = base64::engine::general_purpose::STANDARD.encode(plaintext.as_bytes());
            if !result.iter().any(|(p, _)| p == &b64_std) {
                encoded_variants.push((b64_std, vk_ref.clone()));
            }
            // base64 URL-safe
            let b64_url = base64::engine::general_purpose::URL_SAFE.encode(plaintext.as_bytes());
            if b64_url != base64::engine::general_purpose::STANDARD.encode(plaintext.as_bytes())
                && !result.iter().any(|(p, _)| p == &b64_url)
            {
                encoded_variants.push((b64_url, vk_ref.clone()));
            }
            // hex lowercase
            let hex_lower: String = plaintext.bytes().map(|b| format!("{:02x}", b)).collect();
            if !result.iter().any(|(p, _)| p == &hex_lower) {
                encoded_variants.push((hex_lower, vk_ref.clone()));
            }
        }
        result.extend(encoded_variants);

        // Sort by plaintext length descending (longest match first)
        result.sort_by(|a, b| b.0.len().cmp(&a.0.len()));

        // Remove entries where plaintext is a substring of any VK ref
        // (prevents double-substitution, e.g. plaintext "ea2bfd16" matching inside "VK:LOCAL:ea2bfd16")
        let all_refs: Vec<String> = result.iter().map(|(_, r)| r.clone()).collect();
        result.retain(|(plaintext, _)| {
            !all_refs
                .iter()
                .any(|r| r.contains(plaintext.as_str()) && r != plaintext)
        });

        Some(result)
    }

    pub fn health_check(&self) -> bool {
        let secs = std::env::var("VEILKEY_HEALTH_TIMEOUT")
            .ok()
            .and_then(|v| v.parse::<u64>().ok())
            .unwrap_or(2);
        self.agent
            .get(&format!("{}/health", self.base_url))
            .timeout(std::time::Duration::from_secs(secs))
            .call()
            .map(|r| r.status() == 200)
            .unwrap_or(false)
    }
}

fn resolve_candidates(token: &str) -> Vec<String> {
    if token.starts_with("VK:") || token.starts_with("VE:") {
        let colon_count = token.chars().filter(|&c| c == ':').count();
        if colon_count == 1 {
            if let Some(idx) = token.find(':') {
                return vec![token[idx + 1..].to_string()];
            }
        }
        let parts: Vec<&str> = token.splitn(3, ':').collect();
        if parts.len() == 3 && parts[0] == "VK" {
            return vec![token.to_string(), parts[2].to_string()];
        }
        return vec![token.to_string()];
    }
    vec![token.to_string()]
}
