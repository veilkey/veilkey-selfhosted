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
    session_cookie: Arc<Mutex<Option<String>>>,
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
            session_cookie: Arc::new(Mutex::new(None)),
        }
    }

    pub fn base_url(&self) -> &str {
        &self.base_url
    }

    /// Login to VaultCenter with admin password.
    /// Extracts session cookie from Set-Cookie header for subsequent requests.
    pub fn admin_login(&self, password: &str) -> Result<(), String> {
        let url = format!("{}/api/admin/login", self.base_url);
        let body = serde_json::json!({"password": password});
        match self.agent.post(&url).send_json(&body) {
            Ok(resp) => {
                if resp.status() != 200 {
                    return Err(format!("login failed: HTTP {}", resp.status()));
                }
                // Extract session cookie from Set-Cookie header
                if let Some(set_cookie) = resp.header("set-cookie") {
                    if let Some(cookie_value) = set_cookie.split(';').next() {
                        if let Ok(mut guard) = self.session_cookie.lock() {
                            *guard = Some(cookie_value.to_string());
                        }
                    }
                }
                Ok(())
            }
            Err(e) => Err(format!("login request failed: {}", e)),
        }
    }

    /// Returns the session cookie header value if logged in.
    fn cookie_header(&self) -> Option<String> {
        self.session_cookie.lock().ok()?.clone()
    }

    #[allow(clippy::result_large_err)]
    pub fn raw_get(&self, url: &str) -> Result<ureq::Response, ureq::Error> {
        let mut req = self.agent.get(url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        req.call()
    }

    #[allow(clippy::result_large_err)]
    pub fn raw_get_with_timeout(
        &self,
        url: &str,
        timeout: std::time::Duration,
    ) -> Result<ureq::Response, ureq::Error> {
        let mut req = self.agent.get(url).timeout(timeout);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        req.call()
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

    /// Fetch all secrets via /api/mask-map (single request).
    /// Returns Vec<(plaintext, vk_ref)> sorted by plaintext length descending.
    /// Returns None if the API is unreachable after retries (fail-closed).
    pub fn fetch_all_secrets_mask_map(&self) -> Option<Vec<(String, String)>> {
        let max_retries: u32 = std::env::var("VEILKEY_MASK_FETCH_RETRIES")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(3);

        // Single request to /api/mask-map — server resolves all secrets
        let mut data: Option<serde_json::Value> = None;
        for attempt in 0..max_retries {
            if attempt > 0 {
                let delay = std::time::Duration::from_millis(500 * (1 << attempt));
                eprintln!(
                    "[veilkey] retrying mask-map fetch in {}ms (attempt {}/{})",
                    delay.as_millis(),
                    attempt + 1,
                    max_retries
                );
                std::thread::sleep(delay);
            }
            let mut req = self
                .agent
                .get(&format!("{}/api/mask-map", self.base_url));
            if let Some(cookie) = self.cookie_header() {
                req = req.set("Cookie", &cookie);
            }
            match req.call()
            {
                Ok(resp) => {
                    data = resp.into_json().ok();
                    break;
                }
                Err(e) => {
                    eprintln!(
                        "[veilkey] mask-map fetch failed (attempt {}/{}): {}",
                        attempt + 1,
                        max_retries,
                        e
                    );
                    // No fallback — fail-closed if mask-map is unavailable
                }
            }
        }

        let data = match data {
            Some(d) => d,
            None => {
                eprintln!(
                    "[veilkey] FATAL: could not fetch mask-map after {} attempts — fail-closed",
                    max_retries
                );
                return None;
            }
        };

        let entries = data["entries"].as_array().cloned().unwrap_or_default();
        let mut result: Vec<(String, String)> = Vec::new();
        for entry in &entries {
            let vk_ref = entry["ref"].as_str().unwrap_or_default();
            let value = entry["value"].as_str().unwrap_or_default();
            let trimmed = value.trim_end_matches(['\r', '\n']);
            if !trimmed.is_empty() && !vk_ref.is_empty() {
                result.push((trimmed.to_string(), vk_ref.to_string()));
            }
        }

        enrich_mask_map(&mut result);

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

/// Add encoded variants (base64, hex) to a mask_map, sort by length descending,
/// and remove entries where plaintext is a substring of any VK ref.
pub fn enrich_mask_map(map: &mut Vec<(String, String)>) {
    let mut encoded: Vec<(String, String)> = Vec::new();
    for (plaintext, vk_ref) in map.iter() {
        if plaintext.len() < 8 {
            continue;
        }
        let b64_std = base64::engine::general_purpose::STANDARD.encode(plaintext.as_bytes());
        if !map.iter().any(|(p, _)| p == &b64_std) {
            encoded.push((b64_std.clone(), vk_ref.clone()));
        }
        let b64_url = base64::engine::general_purpose::URL_SAFE.encode(plaintext.as_bytes());
        if b64_url != b64_std && !map.iter().any(|(p, _)| p == &b64_url) {
            encoded.push((b64_url, vk_ref.clone()));
        }
        let hex: String = plaintext.bytes().map(|b| format!("{:02x}", b)).collect();
        if !map.iter().any(|(p, _)| p == &hex) {
            encoded.push((hex, vk_ref.clone()));
        }
    }
    map.extend(encoded);
    map.sort_by(|a, b| b.0.len().cmp(&a.0.len()));

    // Remove entries where plaintext is a substring of any VK ref
    let all_refs: Vec<String> = map.iter().map(|(_, r)| r.clone()).collect();
    map.retain(|(pt, _)| !all_refs.iter().any(|r| r.contains(pt.as_str()) && r != pt));
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
