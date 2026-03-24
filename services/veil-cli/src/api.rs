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
    ve_entries: Arc<Mutex<Vec<(String, String)>>>,
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
            ve_entries: Arc::new(Mutex::new(Vec::new())),
        }
    }

    pub fn base_url(&self) -> &str {
        &self.base_url
    }

    pub fn get_ve_entries(&self) -> Vec<(String, String)> {
        self.ve_entries.lock().unwrap().clone()
    }

    /// Login to VaultCenter with admin password.
    /// Extracts session cookie from Set-Cookie header for subsequent requests.
    pub fn admin_login(&self, password: &str) -> Result<(), String> {
        let url = format!("{}/api/admin/login", self.base_url);
        let body = serde_json::json!({"password": password});
        match self.agent.post(&url).send_json(&body) {
            Ok(resp) => {
                if let Some(set_cookie) = resp.header("set-cookie") {
                    if let Some(cookie_value) = set_cookie.split(';').next() {
                        if let Ok(mut guard) = self.session_cookie.lock() {
                            *guard = Some(cookie_value.to_string());
                        }
                    }
                }
                Ok(())
            }
            Err(ureq::Error::Status(429, _)) => {
                Err("too many attempts — try again later".to_string())
            }
            Err(ureq::Error::Status(code, _)) => {
                Err(format!("login failed (HTTP {})", code))
            }
            Err(_) => Err("cannot reach VeilKey server".to_string()),
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
        let mut req = self
            .agent
            .post(&format!("{}/api/encrypt", self.base_url))
            .set("Content-Type", "application/json");
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        let resp = req
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
        let mut req = self.agent.get(&url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        let resp = req.call().map_err(|_| "resolve failed".to_string())?;

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
            let mut req = self.agent.get(&format!("{}/api/mask-map", self.base_url));
            if let Some(cookie) = self.cookie_header() {
                req = req.set("Cookie", &cookie);
            }
            match req.call() {
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

        if let Some(ve_arr) = data["ve_entries"].as_array() {
            let mut ve: Vec<(String, String)> = Vec::new();
            for entry in ve_arr {
                let ve_ref = entry["ref"].as_str().unwrap_or_default();
                let value = entry["value"].as_str().unwrap_or_default();
                let trimmed = value.trim_end_matches(['\r', '\n']);
                if !trimmed.is_empty() && !ve_ref.is_empty() {
                    ve.push((trimmed.to_string(), ve_ref.to_string()));
                }
            }
            if let Ok(mut guard) = self.ve_entries.lock() {
                *guard = ve;
            }
        }

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
    use std::collections::HashSet;
    let mut seen: HashSet<String> = map.iter().map(|(p, _)| p.clone()).collect();
    let mut encoded: Vec<(String, String)> = Vec::new();
    for (plaintext, vk_ref) in map.iter() {
        if plaintext.len() < 8 {
            continue;
        }
        let b64_std = base64::engine::general_purpose::STANDARD.encode(plaintext.as_bytes());
        if seen.insert(b64_std.clone()) {
            encoded.push((b64_std.clone(), vk_ref.clone()));
        }
        let b64_url = base64::engine::general_purpose::URL_SAFE.encode(plaintext.as_bytes());
        if b64_url != b64_std && seen.insert(b64_url.clone()) {
            encoded.push((b64_url, vk_ref.clone()));
        }
        let hex: String = plaintext.bytes().map(|b| format!("{:02x}", b)).collect();
        if seen.insert(hex.clone()) {
            encoded.push((hex, vk_ref.clone()));
        }
    }
    map.extend(encoded);
    map.sort_by(|a, b| b.0.len().cmp(&a.0.len()));

    // Remove entries where plaintext would corrupt VK/VE ref tokens in output.
    // Only filter if plaintext is literally a substring of the ref format itself
    // (e.g., plaintext "LOCAL" would break "VK:LOCAL:xxx" references).
    map.retain(|(pt, own_ref)| {
        // Skip very short plaintexts that could match ref prefixes
        if pt.len() < 3 {
            return false;
        }
        // Check if this plaintext appears inside its OWN ref (self-corruption)
        if own_ref.contains(pt.as_str()) {
            return false;
        }
        // Check if plaintext matches common ref prefix components
        let pt_upper = pt.to_uppercase();
        if pt_upper == "VK"
            || pt_upper == "VE"
            || pt_upper == "LOCAL"
            || pt_upper == "TEMP"
            || pt_upper == "HOST"
        {
            return false;
        }
        true
    });
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

#[cfg(test)]
mod tests {
    use super::enrich_mask_map;

    // ── Retain filter ───────────────────────────────────────────────

    #[test]
    fn test_enrich_preserves_normal_entries() {
        let mut map = vec![
            ("hkdgh1@dbhost".to_string(), "VK:LOCAL:6da25530".to_string()),
            (
                "my-secret-password".to_string(),
                "VK:LOCAL:abc12345".to_string(),
            ),
        ];
        enrich_mask_map(&mut map);
        assert!(map.iter().any(|(p, _)| p == "hkdgh1@dbhost"));
        assert!(map.iter().any(|(p, _)| p == "my-secret-password"));
    }

    #[test]
    fn test_enrich_removes_self_corruption() {
        let mut map = vec![("6da25530".to_string(), "VK:LOCAL:6da25530".to_string())];
        enrich_mask_map(&mut map);
        assert!(!map.iter().any(|(p, _)| p == "6da25530"));
    }

    #[test]
    fn test_enrich_removes_ref_keywords() {
        let mut map = vec![
            ("LOCAL".to_string(), "VK:LOCAL:abc".to_string()),
            ("TEMP".to_string(), "VK:TEMP:def".to_string()),
            ("VK".to_string(), "VK:LOCAL:ghi".to_string()),
            ("HOST".to_string(), "VK:HOST:jkl".to_string()),
            ("VE".to_string(), "VE:LOCAL:mno".to_string()),
        ];
        enrich_mask_map(&mut map);
        assert!(map.is_empty(), "all ref keyword plaintexts must be removed");
    }

    #[test]
    fn test_enrich_removes_ref_keywords_case_insensitive() {
        let mut map = vec![
            ("local".to_string(), "VK:LOCAL:abc".to_string()),
            ("temp".to_string(), "VK:TEMP:def".to_string()),
            ("Local".to_string(), "VK:LOCAL:ghi".to_string()),
        ];
        enrich_mask_map(&mut map);
        assert!(map.is_empty());
    }

    #[test]
    fn test_enrich_does_not_remove_unrelated_entries() {
        let mut map = vec![
            ("pass".to_string(), "VK:LOCAL:aaa11111".to_string()),
            ("passport-key".to_string(), "VK:LOCAL:bbb22222".to_string()),
        ];
        enrich_mask_map(&mut map);
        assert!(map.iter().any(|(p, _)| p == "pass"));
        assert!(map.iter().any(|(p, _)| p == "passport-key"));
    }

    #[test]
    fn test_enrich_preserves_secret_containing_ref_substring() {
        // "abc12345" contains "abc" which is in ref "VK:LOCAL:abc12345"
        // but "hunter2-password" does NOT appear in its own ref
        let mut map = vec![(
            "hunter2-password".to_string(),
            "VK:LOCAL:abc12345".to_string(),
        )];
        enrich_mask_map(&mut map);
        assert!(map.iter().any(|(p, _)| p == "hunter2-password"));
    }

    #[test]
    fn test_enrich_removes_very_short_plaintexts() {
        let mut map = vec![
            ("ab".to_string(), "VK:LOCAL:xxx".to_string()),
            ("x".to_string(), "VK:LOCAL:yyy".to_string()),
            ("abc".to_string(), "VK:LOCAL:zzz".to_string()), // 3 chars — kept
        ];
        enrich_mask_map(&mut map);
        assert!(
            !map.iter().any(|(p, _)| p == "ab"),
            "2-char plaintext removed"
        );
        assert!(
            !map.iter().any(|(p, _)| p == "x"),
            "1-char plaintext removed"
        );
        assert!(map.iter().any(|(p, _)| p == "abc"), "3-char plaintext kept");
    }

    // ── Sorting ─────────────────────────────────────────────────────

    #[test]
    fn test_enrich_sorted_longest_first() {
        let mut map = vec![
            ("short".to_string(), "VK:LOCAL:aaa".to_string()),
            (
                "a-much-longer-secret".to_string(),
                "VK:LOCAL:bbb".to_string(),
            ),
            ("medium-secret".to_string(), "VK:LOCAL:ccc".to_string()),
        ];
        enrich_mask_map(&mut map);
        let lens: Vec<usize> = map.iter().map(|(p, _)| p.len()).collect();
        for w in lens.windows(2) {
            assert!(w[0] >= w[1], "not sorted longest-first: {:?}", lens);
        }
    }

    #[test]
    fn test_enrich_sorted_with_encoded_variants() {
        let mut map = vec![("my-api-key-value".to_string(), "VK:LOCAL:xyz".to_string())];
        enrich_mask_map(&mut map);
        let lens: Vec<usize> = map.iter().map(|(p, _)| p.len()).collect();
        for w in lens.windows(2) {
            assert!(
                w[0] >= w[1],
                "encoded variants broke sort order: {:?}",
                lens
            );
        }
    }

    // ── Encoded variants ────────────────────────────────────────────

    #[test]
    fn test_enrich_adds_encoded_variants() {
        let mut map = vec![("my-api-key-value".to_string(), "VK:LOCAL:xyz".to_string())];
        enrich_mask_map(&mut map);
        assert!(
            map.len() > 1,
            "must add encoded variants for secrets >= 8 chars"
        );

        let hex: String = "my-api-key-value"
            .bytes()
            .map(|b| format!("{:02x}", b))
            .collect();
        assert!(map.iter().any(|(p, _)| p == &hex), "hex variant missing");

        let b64 = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            "my-api-key-value".as_bytes(),
        );
        assert!(map.iter().any(|(p, _)| p == &b64), "base64 variant missing");
    }

    #[test]
    fn test_enrich_no_encoded_for_short_secrets() {
        let mut map = vec![("short".to_string(), "VK:LOCAL:aaa".to_string())];
        let original_count = map.len();
        enrich_mask_map(&mut map);
        // "short" is 5 chars < 8 — no encoded variants added
        assert_eq!(map.len(), original_count, "no variants for short secrets");
    }

    #[test]
    fn test_enrich_no_duplicate_encoded() {
        // Same plaintext with two refs — encoded variants should only appear once
        let mut map = vec![
            (
                "my-secret-key-value".to_string(),
                "VK:LOCAL:aaa".to_string(),
            ),
            (
                "my-secret-key-value".to_string(),
                "VK:LOCAL:bbb".to_string(),
            ),
        ];
        enrich_mask_map(&mut map);
        let hex: String = "my-secret-key-value"
            .bytes()
            .map(|b| format!("{:02x}", b))
            .collect();
        let hex_count = map.iter().filter(|(p, _)| p == &hex).count();
        assert_eq!(hex_count, 1, "hex variant must appear exactly once");
    }

    // ── Real-world patterns ─────────────────────────────────────────

    #[test]
    fn test_enrich_connection_string_password() {
        let mut map = vec![("p@ssw0rd!123".to_string(), "VK:LOCAL:conn1".to_string())];
        enrich_mask_map(&mut map);
        assert!(
            map.iter().any(|(p, _)| p == "p@ssw0rd!123"),
            "special char password must survive"
        );
    }

    #[test]
    fn test_enrich_aws_key_pattern() {
        let mut map = vec![
            (
                "AKIAIOSFODNN7EXAMPLE".to_string(),
                "VK:LOCAL:aws1".to_string(),
            ),
            (
                "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY".to_string(),
                "VK:LOCAL:aws2".to_string(),
            ),
        ];
        enrich_mask_map(&mut map);
        assert!(map.iter().any(|(p, _)| p == "AKIAIOSFODNN7EXAMPLE"));
        assert!(map
            .iter()
            .any(|(p, _)| p == "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"));
    }

    #[test]
    fn test_enrich_github_token() {
        let token = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef12";
        let mut map = vec![(token.to_string(), "VK:LOCAL:gh1".to_string())];
        enrich_mask_map(&mut map);
        assert!(map.iter().any(|(p, _)| p == token));
    }

    #[test]
    fn test_enrich_many_secrets_performance() {
        // 100 secrets — should not blow up
        let mut map: Vec<(String, String)> = (0..100)
            .map(|i| {
                (
                    format!("secret-value-{:04}-padding", i),
                    format!("VK:LOCAL:{:08x}", i),
                )
            })
            .collect();
        enrich_mask_map(&mut map);
        // All originals survive (none match their own ref)
        assert!(map.iter().any(|(p, _)| p == "secret-value-0000-padding"));
        assert!(map.iter().any(|(p, _)| p == "secret-value-0099-padding"));
        // Sorted longest-first
        let lens: Vec<usize> = map.iter().map(|(p, _)| p.len()).collect();
        for w in lens.windows(2) {
            assert!(w[0] >= w[1]);
        }
    }
}
