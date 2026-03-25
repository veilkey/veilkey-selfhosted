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
            Err(ureq::Error::Status(code, _)) => Err(format!("login failed (HTTP {})", code)),
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

    /// List all global functions.
    pub fn function_list(&self) -> Result<Vec<serde_json::Value>, String> {
        let url = format!("{}/api/functions/global", self.base_url);
        let mut req = self.agent.get(&url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        let resp = req
            .call()
            .map_err(|e| format!("function list failed: {}", e))?;
        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("function list decode failed: {}", e))?;
        let functions = result["functions"].as_array().cloned().unwrap_or_default();
        Ok(functions)
    }

    /// Create a global function by name.
    pub fn function_add(&self, name: &str, command: &str) -> Result<(), String> {
        let url = format!("{}/api/functions/global", self.base_url);
        // Simple hash: first 8 hex chars of name bytes sum
        let hash: u64 = name.bytes().enumerate().fold(0u64, |acc, (i, b)| {
            acc.wrapping_add((b as u64).wrapping_mul(31u64.wrapping_pow(i as u32)))
        });
        let body = serde_json::json!({
            "name": name,
            "command": command,
            "function_hash": format!("{:08x}", hash),
            "category": "",
            "vars_json": "[]"
        });
        let mut req = self
            .agent
            .post(&url)
            .set("Content-Type", "application/json");
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        req.send_json(&body)
            .map_err(|e| format!("function add failed: {}", e))?;
        Ok(())
    }

    /// Delete a global function by name.
    pub fn function_remove(&self, name: &str) -> Result<(), String> {
        let url = format!(
            "{}/api/functions/global/{}",
            self.base_url,
            urlencoding::encode(name)
        );
        let mut req = self.agent.delete(&url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        req.call()
            .map_err(|e| format!("function remove failed: {}", e))?;
        Ok(())
    }

    /// Register an SSH key via the VaultCenter API.
    pub fn ssh_add(
        &self,
        label: &str,
        public_key: &str,
        private_key: Option<&str>,
        key_type: &str,
    ) -> Result<String, String> {
        let url = format!("{}/api/ssh/keys", self.base_url);
        let mut body = serde_json::json!({
            "label": label,
            "public_key": public_key,
            "key_type": key_type,
        });
        if let Some(pk) = private_key {
            body["private_key"] = serde_json::Value::String(pk.to_string());
        }
        let mut req = self
            .agent
            .post(&url)
            .set("Content-Type", "application/json");
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        let resp = req
            .send_json(&body)
            .map_err(|e| format!("ssh add failed: {}", e))?;
        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("ssh add decode failed: {}", e))?;
        let vk_ref = result["ref"]
            .as_str()
            .ok_or("missing ref in response")?
            .to_string();
        Ok(vk_ref)
    }

    /// List all SSH keys (public info only).
    pub fn ssh_list(&self) -> Result<Vec<serde_json::Value>, String> {
        let url = format!("{}/api/ssh/keys", self.base_url);
        let mut req = self.agent.get(&url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        let resp = req.call().map_err(|e| format!("ssh list failed: {}", e))?;
        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("ssh list decode failed: {}", e))?;
        let keys = result["keys"].as_array().cloned().unwrap_or_default();
        Ok(keys)
    }

    /// Get the public key for an SSH key ref.
    pub fn ssh_pubkey(&self, ref_str: &str) -> Result<String, String> {
        let url = format!(
            "{}/api/ssh/keys/{}",
            self.base_url,
            urlencoding::encode(ref_str)
        );
        let mut req = self.agent.get(&url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        let resp = req
            .call()
            .map_err(|e| format!("ssh pubkey failed: {}", e))?;
        let result: serde_json::Value = resp
            .into_json()
            .map_err(|e| format!("ssh pubkey decode failed: {}", e))?;
        result["public_key"]
            .as_str()
            .map(|s| s.to_string())
            .ok_or_else(|| "missing public_key in response".to_string())
    }

    /// Delete an SSH key by ref.
    pub fn ssh_remove(&self, ref_str: &str) -> Result<(), String> {
        let url = format!(
            "{}/api/ssh/keys/{}",
            self.base_url,
            urlencoding::encode(ref_str)
        );
        let mut req = self.agent.delete(&url);
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        req.call()
            .map_err(|e| format!("ssh remove failed: {}", e))?;
        Ok(())
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

    /// Check if VaultCenter is locked by probing /api/mask-map.
    pub fn is_locked(&self) -> bool {
        let url = format!("{}/api/mask-map", self.base_url);
        matches!(
            self.agent
                .get(&url)
                .timeout(std::time::Duration::from_secs(5))
                .call(),
            Err(ureq::Error::Status(503, _))
        )
    }

    /// Unlock VaultCenter with master password (KEK).
    pub fn unlock(&self, password: &str) -> Result<(), String> {
        let url = format!("{}/api/unlock", self.base_url);
        let body = json_password_body(password);
        match self
            .agent
            .post(&url)
            .set("Content-Type", "application/json")
            .send_string(&body)
        {
            Ok(resp) => {
                let result: serde_json::Value = resp.into_json().unwrap_or_default();
                let status = result["status"].as_str().unwrap_or("");
                if status == "unlocked" || status == "already_unlocked" {
                    Ok(())
                } else {
                    Err("unexpected response".to_string())
                }
            }
            Err(ureq::Error::Status(401, _)) => Err("invalid master password".to_string()),
            Err(ureq::Error::Status(429, _)) => {
                Err("too many attempts — try again later".to_string())
            }
            Err(ureq::Error::Status(code, _)) => Err(format!("unlock failed (HTTP {})", code)),
            Err(_) => Err("cannot reach VaultCenter".to_string()),
        }
    }
}

/// Parse an SSH key file and return (key_type, public_key, Option<private_key>).
/// Supports both private key files (extracts public key from comment or generates)
/// and public key files.
pub fn parse_ssh_key(content: &str) -> (String, String, Option<String>) {
    let trimmed = content.trim();

    // Check if it's a public key file (starts with ssh-...)
    if trimmed.starts_with("ssh-ed25519 ")
        || trimmed.starts_with("ssh-rsa ")
        || trimmed.starts_with("ecdsa-sha2-")
    {
        let key_type = if trimmed.starts_with("ssh-ed25519") {
            "ed25519"
        } else if trimmed.starts_with("ssh-rsa") {
            "rsa"
        } else {
            "ecdsa"
        };
        return (key_type.to_string(), trimmed.to_string(), None);
    }

    // It's a private key file — detect type from header
    let key_type = if trimmed.contains("OPENSSH PRIVATE KEY") {
        // Need to check the key data for type; heuristic: check for ed25519 in decoded data
        // or check file size (ed25519 private keys are much smaller)
        if trimmed.len() < 800 {
            "ed25519"
        } else {
            "rsa"
        }
    } else if trimmed.contains("RSA PRIVATE KEY") {
        "rsa"
    } else if trimmed.contains("EC PRIVATE KEY") {
        "ecdsa"
    } else {
        "unknown"
    };

    // Try to find a corresponding .pub file or extract from the last line comment
    // For now, return the private key content and empty public key
    // The caller should provide the public key separately or the server derives it
    (
        key_type.to_string(),
        String::new(),
        Some(trimmed.to_string()),
    )
}

/// Build a JSON `{"password":"..."}` body with proper escaping.
fn json_password_body(password: &str) -> String {
    let mut buf = String::with_capacity(16 + password.len() * 2);
    buf.push_str(r#"{"password":""#);
    for ch in password.chars() {
        match ch {
            '"' => buf.push_str(r#"\""#),
            '\\' => buf.push_str(r"\\"),
            '\n' => buf.push_str(r"\n"),
            '\r' => buf.push_str(r"\r"),
            '\t' => buf.push_str(r"\t"),
            c if c < '\x20' => {
                buf.push_str(&format!(r"\u{:04x}", c as u32));
            }
            c => buf.push(c),
        }
    }
    buf.push_str(r#""}"#);
    buf
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

    // ── Defense: JSON injection in password body ─────────────────────

    #[test]
    fn defense_json_password_nested_escape() {
        let password = r#"\"},{"admin":true"#;
        let body = super::json_password_body(password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("nested escape must produce valid JSON");
        assert_eq!(
            parsed["password"].as_str().unwrap(),
            password,
            "password must be preserved exactly"
        );
        // Must NOT have extra keys injected
        assert!(
            parsed.get("admin").is_none(),
            "injection must not create additional JSON keys"
        );
    }

    #[test]
    fn defense_json_password_unicode_escape_injection() {
        // \u0022 is a JSON unicode escape for double-quote
        let password = r#"\u0022,\u0022admin\u0022:true"#;
        let body = super::json_password_body(password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("unicode escape injection must produce valid JSON");
        assert_eq!(
            parsed["password"].as_str().unwrap(),
            password,
            "unicode escapes in password must be treated as literal characters"
        );
        assert!(
            parsed.get("admin").is_none(),
            "unicode escape injection must not create additional JSON keys"
        );
    }

    #[test]
    fn defense_json_password_very_long() {
        // 1MB password — verify no panic
        let password: String = "A".repeat(1_000_000);
        let body = super::json_password_body(&password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("very long password must produce valid JSON");
        assert_eq!(
            parsed["password"].as_str().unwrap().len(),
            1_000_000,
            "long password must be preserved"
        );
    }

    #[test]
    fn defense_json_password_binary_data() {
        // Binary-ish data with control characters
        let password = "pass\x01\x02\x03\x04\x05\x06\x07";
        let body = super::json_password_body(password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("binary data in password must produce valid JSON");
        // The control chars should be escaped as \u00xx
        let recovered = parsed["password"].as_str().unwrap();
        assert_eq!(
            recovered.len(),
            password.len(),
            "binary password must be preserved (control chars escaped then decoded)"
        );
    }

    #[test]
    fn defense_json_password_null_byte() {
        // Null byte in password
        let password = "pass\x00word";
        let body = super::json_password_body(password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("null byte in password must produce valid JSON");
        let recovered = parsed["password"].as_str().unwrap();
        assert_eq!(
            recovered, password,
            "null byte must be preserved in password"
        );
    }

    #[test]
    fn defense_json_password_all_special_chars() {
        let password = "\"\\/{}\n\r\t";
        let body = super::json_password_body(password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("special chars must produce valid JSON");
        assert_eq!(
            parsed["password"].as_str().unwrap(),
            password,
            "all special chars must round-trip correctly"
        );
    }

    #[test]
    fn defense_json_password_empty() {
        let body = super::json_password_body("");
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("empty password must produce valid JSON");
        assert_eq!(parsed["password"].as_str().unwrap(), "");
    }

    #[test]
    fn defense_json_password_only_structure() {
        // Attempt to create a completely new JSON structure
        let password = r#""},"new_key":"value","x":{"password":"#;
        let body = super::json_password_body(password);
        let parsed: serde_json::Value =
            serde_json::from_str(&body).expect("structural injection must produce valid JSON");
        assert!(
            parsed.get("new_key").is_none(),
            "structural injection must not create new keys"
        );
        assert_eq!(parsed["password"].as_str().unwrap(), password,);
    }

    // ── Defense: resolve_candidates ──────────────────────────────────

    #[test]
    fn defense_resolve_candidates_injection() {
        // Verify resolve_candidates doesn't produce unexpected URLs
        let candidates = super::resolve_candidates("VK:LOCAL:abc123");
        assert!(!candidates.is_empty());
        for c in &candidates {
            assert!(
                !c.contains(".."),
                "resolve candidate must not contain path traversal: {}",
                c
            );
        }
    }

    // ── Dense edge cases ──────────────────────────────────────────

    #[test]
    fn edge_secret_substring_of_own_ref() {
        // Secret "LOCAL" is a substring of its own ref "VK:LOCAL:abcd1234"
        // Must be removed because it matches a ref keyword.
        let mut map = vec![("LOCAL".to_string(), "VK:LOCAL:abcd1234".to_string())];
        enrich_mask_map(&mut map);
        assert!(
            !map.iter().any(|(p, _)| p == "LOCAL"),
            "secret 'LOCAL' must be removed (ref keyword)"
        );
    }

    #[test]
    fn edge_secret_is_self_referencing_vk_format() {
        // Secret is exactly "VK:LOCAL:abcd1234" — same as the ref
        let mut map = vec![(
            "VK:LOCAL:abcd1234".to_string(),
            "VK:LOCAL:abcd1234".to_string(),
        )];
        enrich_mask_map(&mut map);
        assert!(
            !map.iter().any(|(p, _)| p == "VK:LOCAL:abcd1234"),
            "self-referencing secret must be removed (contains own ref)"
        );
    }

    #[test]
    fn edge_secret_only_whitespace() {
        let mut map = vec![("   ".to_string(), "VK:LOCAL:ws123456".to_string())];
        enrich_mask_map(&mut map);
        assert!(
            map.iter().any(|(p, _)| p == "   "),
            "whitespace-only secret should survive enrichment"
        );
    }

    #[test]
    fn edge_secret_with_leading_trailing_newlines() {
        let mut map = vec![("\nhello\n".to_string(), "VK:LOCAL:nl123456".to_string())];
        enrich_mask_map(&mut map);
        assert!(
            map.iter().any(|(p, _)| p == "\nhello\n"),
            "newline-containing secret should survive enrichment"
        );
    }

    #[test]
    fn edge_two_secrets_prefix_relationship() {
        let mut map = vec![
            ("pass".to_string(), "VK:LOCAL:short123".to_string()),
            ("password".to_string(), "VK:LOCAL:long1234".to_string()),
        ];
        enrich_mask_map(&mut map);
        assert!(map.iter().any(|(p, _)| p == "pass"), "'pass' must survive");
        assert!(
            map.iter().any(|(p, _)| p == "password"),
            "'password' must survive"
        );
        let pass_idx = map.iter().position(|(p, _)| p == "pass").unwrap();
        let password_idx = map.iter().position(|(p, _)| p == "password").unwrap();
        assert!(
            password_idx < pass_idx,
            "longer secret must come first: password@{} pass@{}",
            password_idx,
            pass_idx
        );
    }

    #[test]
    fn edge_secret_with_null_bytes() {
        let mut map = vec![("abc\x00def".to_string(), "VK:LOCAL:null1234".to_string())];
        enrich_mask_map(&mut map);
        assert!(
            map.iter().any(|(p, _)| p == "abc\x00def"),
            "null-byte secret should survive enrichment"
        );
    }

    #[test]
    fn edge_empty_mask_map() {
        let mut map: Vec<(String, String)> = Vec::new();
        enrich_mask_map(&mut map);
        assert!(map.is_empty(), "empty map should remain empty");
    }

    #[test]
    fn edge_1000_entries_performance() {
        let mut map: Vec<(String, String)> = (0..1000)
            .map(|i| {
                (
                    format!("secret-value-{:04}-padding-extra", i),
                    format!("VK:LOCAL:{:08x}", i),
                )
            })
            .collect();
        let start = std::time::Instant::now();
        enrich_mask_map(&mut map);
        let elapsed = start.elapsed();
        assert!(map
            .iter()
            .any(|(p, _)| p == "secret-value-0000-padding-extra"));
        assert!(map
            .iter()
            .any(|(p, _)| p == "secret-value-0999-padding-extra"));
        assert!(map.len() > 1000, "encoded variants must be added");
        let lens: Vec<usize> = map.iter().map(|(p, _)| p.len()).collect();
        for w in lens.windows(2) {
            assert!(w[0] >= w[1], "sort violated: {} < {}", w[0], w[1]);
        }
        assert!(
            elapsed.as_secs() < 5,
            "1000 entries took {:?} — too slow",
            elapsed
        );
    }

    #[test]
    fn edge_hex_encoding_short_secret_not_added() {
        let mut map = vec![("hello".to_string(), "VK:LOCAL:hel12345".to_string())];
        enrich_mask_map(&mut map);
        let hex = "68656c6c6f";
        assert!(
            !map.iter().any(|(p, _)| p == hex),
            "hex encoding should NOT be added for secrets < 8 chars"
        );
    }

    #[test]
    fn edge_hex_encoding_long_secret_added() {
        let mut map = vec![("hellowor".to_string(), "VK:LOCAL:hex12345".to_string())];
        enrich_mask_map(&mut map);
        let hex: String = "hellowor".bytes().map(|b| format!("{:02x}", b)).collect();
        assert!(
            map.iter().any(|(p, _)| p == &hex),
            "hex encoding must be added for secrets >= 8 chars"
        );
    }

    #[test]
    fn edge_base64_encoding_long_secret_added() {
        let mut map = vec![("hellowor".to_string(), "VK:LOCAL:b64_1234".to_string())];
        enrich_mask_map(&mut map);
        let b64 = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            "hellowor".as_bytes(),
        );
        assert!(
            map.iter().any(|(p, _)| p == &b64),
            "base64 encoding must be added for secrets >= 8 chars"
        );
    }

    #[test]
    fn edge_base64_encoding_short_secret_not_added() {
        let mut map = vec![("hello".to_string(), "VK:LOCAL:b64short".to_string())];
        enrich_mask_map(&mut map);
        let b64 = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            "hello".as_bytes(),
        );
        assert!(
            !map.iter().any(|(p, _)| p == &b64),
            "base64 encoding should NOT be added for secrets < 8 chars"
        );
    }

    #[test]
    fn edge_secret_shorter_than_3_chars_removed() {
        let mut map = vec![
            ("a".to_string(), "VK:LOCAL:one11111".to_string()),
            ("ab".to_string(), "VK:LOCAL:two22222".to_string()),
        ];
        enrich_mask_map(&mut map);
        assert!(!map.iter().any(|(p, _)| p == "a"), "1-char removed");
        assert!(!map.iter().any(|(p, _)| p == "ab"), "2-char removed");
    }

    #[test]
    fn edge_secret_equal_to_vk_keyword() {
        let mut map = vec![
            ("VK".to_string(), "VK:LOCAL:kw_vk001".to_string()),
            ("VE".to_string(), "VE:LOCAL:kw_ve001".to_string()),
            ("LOCAL".to_string(), "VK:LOCAL:kw_loc01".to_string()),
            ("TEMP".to_string(), "VK:TEMP:kw_tmp01".to_string()),
            ("HOST".to_string(), "VK:HOST:kw_hst01".to_string()),
        ];
        enrich_mask_map(&mut map);
        assert!(
            map.is_empty(),
            "all keyword secrets must be removed, remaining: {:?}",
            map
        );
    }

    // ── parse_ssh_key tests ─────────────────────────────────────────

    #[test]
    fn test_parse_ssh_key_public_ed25519() {
        let content = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host";
        let (kt, pubkey, privkey) = super::parse_ssh_key(content);
        assert_eq!(kt, "ed25519");
        assert!(pubkey.starts_with("ssh-ed25519"));
        assert!(privkey.is_none());
    }

    #[test]
    fn test_parse_ssh_key_public_rsa() {
        let content = "ssh-rsa AAAAB3NzaC1yc2EAAA... user@host";
        let (kt, pubkey, privkey) = super::parse_ssh_key(content);
        assert_eq!(kt, "rsa");
        assert!(pubkey.starts_with("ssh-rsa"));
        assert!(privkey.is_none());
    }

    #[test]
    fn test_parse_ssh_key_private_rsa() {
        let content = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIB...\n-----END RSA PRIVATE KEY-----";
        let (kt, pubkey, privkey) = super::parse_ssh_key(content);
        assert_eq!(kt, "rsa");
        assert!(pubkey.is_empty());
        assert!(privkey.is_some());
    }

    #[test]
    fn test_parse_ssh_key_private_openssh() {
        // Small key content simulating ed25519 (< 800 chars)
        let content =
            "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Blbn...\n-----END OPENSSH PRIVATE KEY-----";
        let (kt, pubkey, privkey) = super::parse_ssh_key(content);
        assert_eq!(kt, "ed25519");
        assert!(pubkey.is_empty());
        assert!(privkey.is_some());
    }
}
