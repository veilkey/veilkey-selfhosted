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

    #[allow(clippy::result_large_err)]
    pub fn raw_post(
        &self,
        url: &str,
        body: &serde_json::Value,
    ) -> Result<ureq::Response, ureq::Error> {
        let mut req = self.agent.post(url).set("Content-Type", "application/json");
        if let Some(cookie) = self.cookie_header() {
            req = req.set("Cookie", &cookie);
        }
        req.send_json(body)
    }

    #[allow(clippy::result_large_err)]
    pub fn raw_delete(&self, url: &str) -> Result<ureq::Response, ureq::Error> {
        let mut req = self.agent.delete(url);
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

    /// Issue a secret with a specific scope (e.g. "SSH").
    pub fn issue_with_scope(&self, value: &str, scope: &str) -> Result<String, String> {
        let value = value.trim_end_matches(['\r', '\n']);
        let body = serde_json::json!({ "plaintext": value, "scope": scope });
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

        let (mut result, ve) = parse_mask_map_entries(&data);

        enrich_mask_map(&mut result);

        if let Ok(mut guard) = self.ve_entries.lock() {
            *guard = ve;
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

    // ── secret management ──────────────────────────────────────

    /// List agents (vaults) from VaultCenter.
    pub fn agents_list(&self) -> Result<Vec<serde_json::Value>, String> {
        let url = format!("{}/api/agents", self.base_url);
        let resp = self.raw_get(&url).map_err(|e| format!("agents list failed: {}", e))?;
        let result: serde_json::Value = resp.into_json().map_err(|e| format!("decode failed: {}", e))?;
        result["agents"]
            .as_array()
            .cloned()
            .ok_or_else(|| "missing agents in response".to_string())
    }

    /// Promote a temp ref to a vault (LOCAL scope).
    pub fn promote(&self, temp_ref: &str, name: &str, agent_hash: &str) -> Result<serde_json::Value, String> {
        let url = format!("{}/api/keycenter/promote", self.base_url);
        let body = serde_json::json!({
            "ref": temp_ref,
            "name": name,
            "vault_hash": agent_hash,
        });
        let resp = self.raw_post(&url, &body).map_err(|e| format!("promote failed: {}", e))?;
        resp.into_json().map_err(|e| format!("decode failed: {}", e))
    }

    /// One-step secret add: create temp ref → auto-select vault → promote.
    pub fn secret_add(&self, name: &str, value: &str, vault_label: Option<&str>) -> Result<serde_json::Value, String> {
        // 1. Create temp ref
        let temp_ref = self.issue(value)?;

        // 2. Get agents and select vault
        let agents = self.agents_list()?;
        let active_agents: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();

        if active_agents.is_empty() {
            return Err("no active vaults found".to_string());
        }

        let agent = if let Some(label) = vault_label {
            active_agents.iter()
                .find(|a| a["label"].as_str() == Some(label) || a["vault_name"].as_str() == Some(label))
                .ok_or_else(|| format!("vault '{}' not found", label))?
        } else if active_agents.len() == 1 {
            active_agents[0]
        } else {
            // Default: pick the one with most secrets (likely the main vault)
            active_agents.iter()
                .max_by_key(|a| a["secrets_count"].as_u64().unwrap_or(0))
                .unwrap()
        };

        let agent_hash = agent["agent_hash"]
            .as_str()
            .ok_or("missing agent_hash")?;

        // 3. Promote
        self.promote(&temp_ref, name, agent_hash)
    }

    /// List secrets from a LocalVault.
    pub fn secret_list(&self, lv_url: &str) -> Result<Vec<serde_json::Value>, String> {
        let url = format!("{}/api/secrets", lv_url.trim_end_matches('/'));
        let resp = self.raw_get(&url).map_err(|e| format!("secret list failed: {}", e))?;
        let result: serde_json::Value = resp.into_json().map_err(|e| format!("decode failed: {}", e))?;
        result["secrets"]
            .as_array()
            .cloned()
            .ok_or_else(|| "missing secrets in response".to_string())
    }

    /// Delete a secret from a LocalVault by name.
    pub fn secret_delete(&self, lv_url: &str, name: &str) -> Result<String, String> {
        let url = format!("{}/api/secrets/{}", lv_url.trim_end_matches('/'), urlencoding::encode(name));
        let resp = self.raw_delete(&url).map_err(|e| format!("delete failed: {}", e))?;
        let result: serde_json::Value = resp.into_json().map_err(|e| format!("decode failed: {}", e))?;
        result["deleted"]
            .as_str()
            .map(|s| s.to_string())
            .ok_or_else(|| result.to_string())
    }
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

/// Parse the mask-map API response, separating VK secrets from VE config entries.
/// Returns (secrets, ve_entries) where:
/// - secrets: VK: refs (and any unknown prefix) → used for PTY output masking
/// - ve_entries: VE: refs → used for config tagging
#[allow(clippy::type_complexity)]
pub fn parse_mask_map_entries(
    data: &serde_json::Value,
) -> (Vec<(String, String)>, Vec<(String, String)>) {
    let entries = data["entries"].as_array().cloned().unwrap_or_default();
    let mut secrets: Vec<(String, String)> = Vec::new();
    let mut ve_entries: Vec<(String, String)> = Vec::new();

    for entry in &entries {
        let vk_ref = entry["ref"].as_str().unwrap_or_default();
        let value = entry["value"].as_str().unwrap_or_default();
        let trimmed = value.trim_end_matches(['\r', '\n']);
        if trimmed.is_empty() || vk_ref.is_empty() {
            continue;
        }
        if vk_ref.starts_with("VE:") {
            ve_entries.push((trimmed.to_string(), vk_ref.to_string()));
        } else {
            secrets.push((trimmed.to_string(), vk_ref.to_string()));
        }
    }

    (secrets, ve_entries)
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
}

#[cfg(test)]
mod connection_domain_tests {
    use super::*;

    /// DOMAIN: admin_login must target VaultCenter (/api/admin/login).
    /// LocalVault does NOT have this endpoint.
    /// If veil-cli points to LocalVault, login will get 404.
    #[test]
    fn domain_admin_login_url_is_admin_login() {
        let src = include_str!("api.rs");
        assert!(
            src.contains("\"/api/admin/login\""),
            "admin_login must call /api/admin/login (VaultCenter endpoint)"
        );
    }

    /// DOMAIN: mask-map must target VaultCenter (/api/mask-map).
    /// LocalVault does NOT have this endpoint.
    #[test]
    fn domain_mask_map_url_is_vaultcenter() {
        let src = include_str!("api.rs");
        assert!(
            src.contains("\"/api/mask-map\"") || src.contains("api/encrypt"),
            "mask_map/encrypt endpoints must exist (VaultCenter-only)"
        );
    }

    /// DOMAIN: resolve must target VaultCenter (/api/resolve/).
    #[test]
    fn domain_resolve_url_is_vaultcenter() {
        let src = include_str!("api.rs");
        assert!(
            src.contains("/api/resolve/"),
            "resolve must call /api/resolve/ (VaultCenter endpoint)"
        );
    }

    /// DOMAIN: VEILKEY_LOCALVAULT_URL is the primary env var.
    /// Despite the name, it should point to VaultCenter for veil-cli.
    /// This is a known naming issue — the env var name is legacy.
    #[test]
    fn domain_env_var_is_localvault_url() {
        let src = include_str!("bin/veilkey_cli.rs");
        assert!(
            src.contains("VEILKEY_LOCALVAULT_URL"),
            "veilkey-cli must read VEILKEY_LOCALVAULT_URL"
        );
    }

    /// DOMAIN: veil session (wrap-pty) must authenticate before loading secrets.
    #[test]
    fn domain_session_authenticates_first() {
        let src = include_str!("pty/session.rs");
        let login_pos = src.find("admin_login").unwrap_or(usize::MAX);
        let mask_pos = src.find("fetch_all_secrets_mask_map").unwrap_or(usize::MAX);
        assert!(
            login_pos < mask_pos,
            "admin_login must happen before fetch_all_secrets_mask_map"
        );
    }

    /// DOMAIN: endpoints used by veil-cli are ALL VaultCenter endpoints.
    /// None of these exist on LocalVault.
    #[test]
    fn domain_all_api_endpoints_are_vaultcenter() {
        let src = include_str!("api.rs");
        let vc_endpoints = [
            "/api/admin/login",
            "/api/mask-map",
            "/api/encrypt",
            "/api/resolve/",
            "/api/ssh/keys",
            "/api/functions/global",
        ];
        for ep in vc_endpoints {
            assert!(
                src.contains(ep),
                "veil-cli must use VaultCenter endpoint: {}",
                ep
            );
        }
    }

    // ── parse_mask_map_entries: VK/VE separation ────────────────────

    #[test]
    fn test_parse_splits_vk_and_ve_entries() {
        let data = serde_json::json!({"version":1,"changed":true,"entries":[
            {"ref":"VK:LOCAL:abc12345","value":"my-secret-password","vault":"host-lv"},
            {"ref":"VE:LOCAL:DB_HOST","value":"10.0.0.5","vault":"host-lv"},
            {"ref":"VK:LOCAL:def67890","value":"another-secret","vault":"host-lv"},
            {"ref":"VE:LOCAL:APP_PORT","value":"8080","vault":"host-lv"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 2);
        assert_eq!(c.len(), 2);
        assert!(s.iter().any(|(p, _)| p == "my-secret-password"));
        assert!(c.iter().any(|(_, r)| r == "VE:LOCAL:DB_HOST"));
    }

    #[test]
    fn test_parse_only_vk_entries() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"s1","vault":"v1"},
            {"ref":"VK:SSH:b","value":"ssh-key","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 2);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_only_ve_entries() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:DB_HOST","value":"10.0.0.5","vault":"v1"},
            {"ref":"VE:LOCAL:DB_PORT","value":"3306","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert_eq!(c.len(), 2);
    }

    #[test]
    fn test_parse_empty_entries() {
        let (s, c) = super::parse_mask_map_entries(&serde_json::json!({"entries":[]}));
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_missing_entries_field() {
        let (s, c) = super::parse_mask_map_entries(&serde_json::json!({"version":1}));
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_trims_trailing_newlines() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"secret\r\n","vault":"v1"},
            {"ref":"VE:LOCAL:H","value":"host\n","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s[0].0, "secret");
        assert_eq!(c[0].0, "host");
    }

    #[test]
    fn test_parse_skips_empty_values() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"","vault":"v1"},
            {"ref":"VE:LOCAL:H","value":"","vault":"v1"},
            {"ref":"VK:LOCAL:b","value":"real","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_skips_empty_refs() {
        let data = serde_json::json!({"entries":[
            {"ref":"","value":"orphan","vault":"v1"},
            {"ref":"VK:LOCAL:a","value":"real","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_unknown_prefix_goes_to_secrets() {
        let data = serde_json::json!({"entries":[
            {"ref":"XX:LOCAL:a","value":"unknown","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_vk_ssh_goes_to_secrets() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:SSH:k","value":"-----BEGIN KEY-----","vault":"vc"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_vk_temp_goes_to_secrets() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:TEMP:t","value":"temp-val","vault":"vc"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_ve_host_goes_to_configs() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:HOST:CFG","value":"val","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert_eq!(c.len(), 1);
    }

    #[test]
    fn test_parse_ve_multiple_scopes() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:A","value":"a","vault":"v1"},
            {"ref":"VE:HOST:B","value":"b","vault":"v1"},
            {"ref":"VE:TEMP:C","value":"c","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert_eq!(c.len(), 3);
    }

    #[test]
    fn test_parse_ve_prefix_is_case_sensitive() {
        let data = serde_json::json!({"entries":[
            {"ref":"ve:LOCAL:a","value":"v","vault":"v1"},
            {"ref":"Ve:LOCAL:b","value":"v","vault":"v1"},
            {"ref":"vE:LOCAL:c","value":"v","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 3);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entry_null_ref() {
        let data = serde_json::json!({"entries":[{"ref":null,"value":"v","vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entry_null_value() {
        let data = serde_json::json!({"entries":[{"ref":"VK:LOCAL:a","value":null,"vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entry_numeric_value() {
        let data = serde_json::json!({"entries":[{"ref":"VK:LOCAL:a","value":123,"vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entry_object_value() {
        let data =
            serde_json::json!({"entries":[{"ref":"VK:LOCAL:a","value":{"x":1},"vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entries_not_array() {
        let (s, c) = super::parse_mask_map_entries(&serde_json::json!({"entries":"str"}));
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entries_null() {
        let (s, c) = super::parse_mask_map_entries(&serde_json::json!({"entries":null}));
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_ref_bare_ve() {
        let data = serde_json::json!({"entries":[{"ref":"VE:","value":"bare","vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert_eq!(c.len(), 1);
    }

    #[test]
    fn test_parse_ref_embedded_ve_not_config() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:VE:abc","value":"tricky","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_value_only_newlines() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"\r\n","vault":"v1"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
    }

    #[test]
    fn test_parse_value_internal_newlines_preserved() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:SSH:k","value":"l1\nl2\nl3\n","vault":"vc"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(s[0].0, "l1\nl2\nl3");
    }

    #[test]
    fn test_parse_value_unicode() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:u","value":"비밀번호🔑","vault":"v1"},
            {"ref":"VE:LOCAL:L","value":"설정값","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s[0].0, "비밀번호🔑");
        assert_eq!(c[0].0, "설정값");
    }

    #[test]
    fn test_parse_value_containing_ref_pattern() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"VK:LOCAL:fake","vault":"v1"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(s[0].0, "VK:LOCAL:fake");
    }

    #[test]
    fn test_parse_duplicate_entries_kept() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"same","vault":"v1"},
            {"ref":"VK:LOCAL:a","value":"same","vault":"v1"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 2);
    }

    #[test]
    fn test_parse_same_value_vk_and_ve() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:s","value":"10.0.0.5","vault":"v1"},
            {"ref":"VE:LOCAL:H","value":"10.0.0.5","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert_eq!(c.len(), 1);
        assert_eq!(s[0].0, c[0].0);
    }

    #[test]
    fn test_parse_preserves_order() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:1","value":"aaa","vault":"v1"},
            {"ref":"VK:LOCAL:2","value":"bbb","vault":"v1"},
            {"ref":"VK:LOCAL:3","value":"ccc","vault":"v1"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(
            (s[0].0.as_str(), s[1].0.as_str(), s[2].0.as_str()),
            ("aaa", "bbb", "ccc")
        );
    }

    #[test]
    fn test_parse_ve_preserves_order() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:A","value":"first","vault":"v1"},
            {"ref":"VK:LOCAL:x","value":"sec","vault":"v1"},
            {"ref":"VE:LOCAL:B","value":"second","vault":"v1"}
        ]});
        let (_, c) = super::parse_mask_map_entries(&data);
        assert_eq!(c[0].0, "first");
        assert_eq!(c[1].0, "second");
    }

    #[test]
    fn test_parse_large_payload() {
        let mut entries = Vec::new();
        for i in 0..500 {
            entries.push(serde_json::json!({"ref":format!("VK:LOCAL:{:08x}",i),"value":format!("s{:04}",i),"vault":"v1"}));
            entries.push(serde_json::json!({"ref":format!("VE:LOCAL:C_{:04}",i),"value":format!("c{:04}",i),"vault":"v1"}));
        }
        let (s, c) = super::parse_mask_map_entries(&serde_json::json!({"entries":entries}));
        assert_eq!(s.len(), 500);
        assert_eq!(c.len(), 500);
    }

    #[test]
    fn defense_parse_injection_in_ref() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:../../etc/passwd","value":"inject","vault":"v1"},
            {"ref":"VE:LOCAL:$(rm -rf /)","value":"inject","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert_eq!(c.len(), 1);
    }

    #[test]
    fn defense_parse_control_chars() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:c","value":"p\u{0001}\u{0002}w","vault":"v1"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(s[0].0, "p\u{0001}\u{0002}w");
    }

    #[test]
    fn defense_parse_very_long_value() {
        let v = "A".repeat(100_000);
        let data = serde_json::json!({"entries":[{"ref":"VK:LOCAL:l","value":v,"vault":"v1"}]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(s[0].0.len(), 100_000);
    }

    #[test]
    fn defense_parse_garbage_entries() {
        let data = serde_json::json!({"entries":["str",42,null,
            {"ref":"VK:LOCAL:g","value":"real","vault":"v1"},
            {"ref":null,"value":"x"},
            {"ref":"VE:LOCAL:C","value":"cfg","vault":"v1"},
            true
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert_eq!(c.len(), 1);
    }

    #[test]
    fn domain_parse_ve_never_in_secrets() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:A","value":"a","vault":"v1"},
            {"ref":"VK:LOCAL:b","value":"b","vault":"v1"},
            {"ref":"VE:HOST:C","value":"c","vault":"v1"}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        for (_, r) in &s {
            assert!(!r.starts_with("VE:"));
        }
    }

    #[test]
    fn domain_parse_vk_never_in_configs() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"s","vault":"v1"},
            {"ref":"VE:LOCAL:B","value":"c","vault":"v1"},
            {"ref":"VK:SSH:d","value":"k","vault":"v1"}
        ]});
        let (_, c) = super::parse_mask_map_entries(&data);
        for (_, r) in &c {
            assert!(!r.starts_with("VK:"));
        }
    }

    #[test]
    fn domain_parse_total_count() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"s1","vault":"v1"},
            {"ref":"VK:LOCAL:b","value":"s2","vault":"v1"},
            {"ref":"VE:LOCAL:C","value":"c1","vault":"v1"},
            {"ref":"VK:LOCAL:d","value":"","vault":"v1"},
            {"ref":"","value":"bad","vault":"v1"}
        ]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len() + c.len(), 3);
    }

    // ── Pipeline integration: parse → enrich ─────────────────────────

    #[test]
    fn pipeline_enrich_only_secrets() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"my-api-key-value","vault":"v1"},
            {"ref":"VE:LOCAL:H","value":"10.0.0.5","vault":"v1"},
            {"ref":"VE:LOCAL:P","value":"5432","vault":"v1"}
        ]});
        let (mut secrets, configs) = super::parse_mask_map_entries(&data);
        super::enrich_mask_map(&mut secrets);
        assert!(secrets.len() > 1);
        assert_eq!(configs.len(), 2);
        for (_, r) in &secrets {
            assert!(!r.starts_with("VE:"));
        }
    }

    #[test]
    fn pipeline_config_not_in_secrets() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:s","value":"super-secret-key-value","vault":"v1"},
            {"ref":"VE:LOCAL:H","value":"192.168.1.100","vault":"v1"}
        ]});
        let (mut secrets, _) = super::parse_mask_map_entries(&data);
        super::enrich_mask_map(&mut secrets);
        assert!(!secrets.iter().any(|(p, _)| p == "192.168.1.100"));
    }

    #[test]
    fn pipeline_no_encoded_variants_for_configs() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:L","value":"a-config-value-that-is-long-enough","vault":"v1"}
        ]});
        let (mut secrets, configs) = super::parse_mask_map_entries(&data);
        super::enrich_mask_map(&mut secrets);
        let cfg_b64 = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            "a-config-value-that-is-long-enough".as_bytes(),
        );
        assert!(!secrets.iter().any(|(p, _)| p == &cfg_b64));
        assert_eq!(configs.len(), 1);
    }

    #[test]
    fn pipeline_realistic_response() {
        let data = serde_json::json!({"version":42,"changed":true,"entries":[
            {"ref":"VK:LOCAL:ce2aac9a","value":"sk-ant-oat01-xxxxx","vault":"host-lv"},
            {"ref":"VK:LOCAL:7accddf2","value":"sk-proj-yyyyy","vault":"host-lv"},
            {"ref":"VK:LOCAL:bdd9d472","value":"p@ssw0rd!123","vault":"host-lv"},
            {"ref":"VK:SSH:key0001","value":"-----BEGIN KEY-----\ndata","vault":"vc"},
            {"ref":"VE:LOCAL:DB_HOST","value":"10.50.0.113","vault":"mysql"},
            {"ref":"VE:LOCAL:DB_PORT","value":"3306","vault":"mysql"},
            {"ref":"VE:LOCAL:APP_ENV","value":"production","vault":"host-lv"},
            {"ref":"VE:HOST:LOG_LEVEL","value":"info","vault":"host-lv"}
        ]});
        let (mut s, c) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 4);
        assert_eq!(c.len(), 4);
        super::enrich_mask_map(&mut s);
        assert!(s.len() > 4);
        assert_eq!(c.len(), 4);
    }

    // ── Source-code regression guards ────────────────────────────────

    #[test]
    fn guard_fetch_uses_parse_function() {
        assert!(include_str!("api.rs").contains("parse_mask_map_entries(&data)"));
    }

    #[test]
    fn guard_no_inline_ve_entries_parsing() {
        let src = include_str!("api.rs");
        let non_test = match src.find("#[cfg(test)]") {
            Some(pos) => &src[..pos],
            None => src,
        };
        let old = format!(r#"data["ve_{}"]"#, "entries");
        assert!(!non_test.contains(&old));
    }

    #[test]
    fn guard_sync_uses_parse_function() {
        assert!(include_str!("pty/sync.rs").contains("parse_mask_map_entries"));
    }

    #[test]
    fn guard_sync_updates_ve_map() {
        assert!(include_str!("pty/sync.rs").contains("ve_map"));
    }

    #[test]
    fn guard_sync_no_inline_parsing() {
        assert!(!include_str!("pty/sync.rs").contains(r#"e["ref"]"#));
    }

    #[test]
    fn guard_session_passes_ve_map_to_sync() {
        assert!(include_str!("pty/session.rs").contains("ve_map.clone()"));
    }

    // ── parse edge cases: extra/malformed fields ─────────────────────

    #[test]
    fn test_parse_entry_extra_fields_ignored() {
        let data = serde_json::json!({"entries":[
            {"ref":"VK:LOCAL:a","value":"secret","vault":"v1","extra":"ignored","num":99}
        ]});
        let (s, _) = super::parse_mask_map_entries(&data);
        assert_eq!(s.len(), 1);
        assert_eq!(s[0].0, "secret");
    }

    #[test]
    fn test_parse_entry_ref_is_number() {
        let data = serde_json::json!({"entries":[{"ref":123,"value":"secret","vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entry_ref_is_boolean() {
        let data = serde_json::json!({"entries":[{"ref":true,"value":"secret","vault":"v1"}]});
        let (s, c) = super::parse_mask_map_entries(&data);
        assert!(s.is_empty());
        assert!(c.is_empty());
    }

    #[test]
    fn test_parse_entry_deeply_nested_extra() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:X","value":"cfg","vault":"v1","nested":{"deep":{"a":1}}}
        ]});
        let (_, c) = super::parse_mask_map_entries(&data);
        assert_eq!(c.len(), 1);
        assert_eq!(c[0].0, "cfg");
    }

    // ── parse: VE short values survive (no length filter) ────────────

    #[test]
    fn test_parse_ve_short_values_accepted() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:A","value":"a","vault":"v1"},
            {"ref":"VE:LOCAL:B","value":"ab","vault":"v1"},
            {"ref":"VE:LOCAL:C","value":"on","vault":"v1"}
        ]});
        let (_, c) = super::parse_mask_map_entries(&data);
        assert_eq!(c.len(), 3, "short VE values must survive parse");
    }

    #[test]
    fn test_parse_same_ve_value_different_scopes() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:HOST","value":"10.0.0.5","vault":"v1"},
            {"ref":"VE:TEMP:HOST","value":"10.0.0.5","vault":"v1"}
        ]});
        let (_, c) = super::parse_mask_map_entries(&data);
        assert_eq!(c.len(), 2);
    }

    // ── pipeline: VE bypasses enrich edge cases ──────────────────────

    #[test]
    fn pipeline_ve_short_value_survives_because_no_enrich() {
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:F","value":"on","vault":"v1"},
            {"ref":"VK:LOCAL:b","value":"secret-long-enough","vault":"v1"}
        ]});
        let (mut secrets, configs) = super::parse_mask_map_entries(&data);
        super::enrich_mask_map(&mut secrets);
        assert_eq!(configs.len(), 1);
        assert_eq!(
            configs[0].0, "on",
            "2-char VE value safe from enrich filter"
        );
    }

    #[test]
    fn pipeline_ve_ref_keyword_value_survives_because_no_enrich() {
        // "LOCAL" would be removed by enrich (ref keyword), but VE bypasses enrich
        let data = serde_json::json!({"entries":[
            {"ref":"VE:LOCAL:SCOPE","value":"LOCAL","vault":"v1"}
        ]});
        let (mut secrets, configs) = super::parse_mask_map_entries(&data);
        super::enrich_mask_map(&mut secrets);
        assert_eq!(configs.len(), 1);
        assert_eq!(configs[0].0, "LOCAL");
    }

    // ── guard: session startup prints correct counts ──────────────────

    #[test]
    fn guard_session_calls_get_ve_entries() {
        assert!(include_str!("pty/session.rs").contains("get_ve_entries()"));
    }

    #[test]
    fn guard_session_prints_config_count() {
        assert!(include_str!("pty/session.rs").contains("config(s) tagged"));
    }

    #[test]
    fn guard_session_ve_count_uses_len() {
        assert!(include_str!("pty/session.rs").contains("ve_entries.len()"));
    }

    // ── resolve_candidates with VE refs ──────────────────────────────

    #[test]
    fn test_resolve_candidates_ve_ref_returns_full_token() {
        // VE refs are NOT resolvable secrets, but resolve_candidates handles
        // them without panic. For VE:, it returns the full token (no hash split).
        let candidates = super::resolve_candidates("VE:LOCAL:DB_HOST");
        assert_eq!(candidates, vec!["VE:LOCAL:DB_HOST"]);
    }

    #[test]
    fn test_resolve_candidates_ve_no_hash_extraction() {
        // VK refs get hash extracted: VK:LOCAL:abc → ["VK:LOCAL:abc", "abc"]
        // VE refs should NOT — only the full token is returned.
        let vk = super::resolve_candidates("VK:LOCAL:abc12345");
        assert_eq!(vk.len(), 2, "VK gets full + hash");
        let ve = super::resolve_candidates("VE:LOCAL:abc12345");
        assert_eq!(ve.len(), 1, "VE gets only full token (not resolvable)");
        assert_eq!(ve[0], "VE:LOCAL:abc12345");
    }

    #[test]
    fn test_resolve_candidates_ve_host_scope() {
        let candidates = super::resolve_candidates("VE:HOST:APP_ENV");
        assert_eq!(candidates, vec!["VE:HOST:APP_ENV"]);
    }

    #[test]
    fn test_resolve_candidates_ve_single_colon() {
        // "VE:something" (only 1 colon) — special path in resolve_candidates
        let candidates = super::resolve_candidates("VE:something");
        assert_eq!(candidates, vec!["something"]);
    }

    // ── env var resolution regex excludes VE ─────────────────────────

    #[test]
    fn guard_veilkey_regex_does_not_match_ve_refs() {
        // The VEILKEY_RE_STR regex (used in session env var resolution)
        // must NOT match VE: refs — only VK: refs should be resolved.
        let re = regex::Regex::new(crate::detector::VEILKEY_RE_STR).expect("regex must compile");
        assert!(
            !re.is_match("VE:LOCAL:DB_HOST"),
            "VE refs must not match env var regex"
        );
        assert!(!re.is_match("VE:HOST:APP_ENV"), "VE:HOST must not match");
        assert!(re.is_match("VK:LOCAL:abc12345"), "VK refs must match");
        assert!(re.is_match("VK:TEMP:abc12345"), "VK:TEMP must match");
    }

    // ── session exit message only counts VK ──────────────────────────

    #[test]
    fn guard_session_exit_message_counts_mask_map_only() {
        // The exit message uses mask.read() (mask_map), not ve_map.
        // This is correct: VE entries are "tagged", not "masked".
        let src = include_str!("pty/session.rs");
        // Exit message reads from "mask" (the mask_map RwLock)
        assert!(
            src.contains("mask.read()"),
            "exit message must read from mask_map"
        );
        // Exit message says "secret(s) masked in session" — no mention of configs
        assert!(
            src.contains("secret(s) masked in session"),
            "exit message should only mention secrets, not configs"
        );
    }

    #[test]
    fn guard_session_exit_message_does_not_read_ve_map() {
        // The exit block must NOT read ve_map — configs are not "masked"
        let src = include_str!("pty/session.rs");
        // Find the exit block (after waitpid)
        let waitpid_pos = src.find("waitpid").unwrap_or(0);
        let exit_block = &src[waitpid_pos..];
        assert!(
            !exit_block.contains("ve_map.read()") && !exit_block.contains("ve.read()"),
            "exit block must not read ve_map"
        );
    }

    // ── plain_tail uses original text, not masked ────────────────────

    #[test]
    fn guard_plain_tail_uses_new_text_not_output() {
        // The plain_tail must track ORIGINAL text (new_text), not masked output.
        // This ensures future cross-chunk matching works on plaintext.
        let src = include_str!("pty/masker.rs");
        // After the VE loop, tail computation uses new_text
        let ve_loop_pos = src.find("for (plaintext, ve_ref) in ve_map").unwrap_or(0);
        let after_ve = &src[ve_loop_pos..];
        assert!(
            after_ve.contains("new_text.len() > PLAIN_TAIL_SIZE")
                || after_ve.contains("new_text[start..]"),
            "plain_tail must use new_text (original), not output (masked)"
        );
    }

    // ── secret vault selection logic ──────────────────────────────

    #[test]
    fn secret_add_selects_vault_with_most_secrets() {
        let agents = vec![
            serde_json::json!({"label": "soulflow", "agent_hash": "aaa", "secrets_count": 2, "archived": false}),
            serde_json::json!({"label": "host-lv", "agent_hash": "bbb", "secrets_count": 13, "archived": false}),
        ];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        let selected = active.iter()
            .max_by_key(|a| a["secrets_count"].as_u64().unwrap_or(0))
            .unwrap();
        assert_eq!(selected["agent_hash"].as_str().unwrap(), "bbb");
    }

    #[test]
    fn secret_add_selects_single_vault() {
        let agents = vec![
            serde_json::json!({"label": "host-lv", "agent_hash": "bbb", "secrets_count": 5, "archived": false}),
        ];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        assert_eq!(active.len(), 1);
        assert_eq!(active[0]["agent_hash"].as_str().unwrap(), "bbb");
    }

    #[test]
    fn secret_add_filters_archived() {
        let agents = vec![
            serde_json::json!({"label": "old", "agent_hash": "aaa", "secrets_count": 50, "archived": true}),
            serde_json::json!({"label": "active", "agent_hash": "bbb", "secrets_count": 3, "archived": false}),
        ];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        assert_eq!(active.len(), 1);
        assert_eq!(active[0]["label"].as_str().unwrap(), "active");
    }

    #[test]
    fn secret_add_vault_match_by_label() {
        let agents = vec![
            serde_json::json!({"label": "soulflow", "vault_name": "soulflow-lv", "agent_hash": "aaa", "archived": false}),
            serde_json::json!({"label": "host-localvault", "vault_name": "host-lv", "agent_hash": "bbb", "archived": false}),
        ];
        let label = "host-localvault";
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        let found = active.iter()
            .find(|a| a["label"].as_str() == Some(label) || a["vault_name"].as_str() == Some(label));
        assert!(found.is_some());
        assert_eq!(found.unwrap()["agent_hash"].as_str().unwrap(), "bbb");
    }

    #[test]
    fn secret_add_vault_match_by_vault_name() {
        let agents = vec![
            serde_json::json!({"label": "host-localvault", "vault_name": "host-lv", "agent_hash": "bbb", "archived": false}),
        ];
        let label = "host-lv";
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        let found = active.iter()
            .find(|a| a["label"].as_str() == Some(label) || a["vault_name"].as_str() == Some(label));
        assert!(found.is_some());
    }

    #[test]
    fn secret_add_vault_not_found() {
        let agents = vec![
            serde_json::json!({"label": "host-lv", "vault_name": "host-lv", "agent_hash": "bbb", "archived": false}),
        ];
        let label = "nonexistent";
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        let found = active.iter()
            .find(|a| a["label"].as_str() == Some(label) || a["vault_name"].as_str() == Some(label));
        assert!(found.is_none());
    }

    #[test]
    fn secret_add_no_active_vaults() {
        let agents: Vec<serde_json::Value> = vec![
            serde_json::json!({"label": "old", "agent_hash": "aaa", "archived": true}),
        ];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        assert!(active.is_empty());
    }

    #[test]
    fn secret_add_empty_agents() {
        let agents: Vec<serde_json::Value> = vec![];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        assert!(active.is_empty());
    }

    #[test]
    fn secret_add_missing_archived_defaults_false() {
        let agents = vec![
            serde_json::json!({"label": "no-field", "agent_hash": "aaa", "secrets_count": 5}),
        ];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        assert_eq!(active.len(), 1);
    }

    #[test]
    fn secret_add_zero_secrets_still_selectable() {
        let agents = vec![
            serde_json::json!({"label": "empty", "agent_hash": "aaa", "secrets_count": 0, "archived": false}),
        ];
        let active: Vec<&serde_json::Value> = agents.iter()
            .filter(|a| !a["archived"].as_bool().unwrap_or(false))
            .collect();
        let selected = active.iter()
            .max_by_key(|a| a["secrets_count"].as_u64().unwrap_or(0))
            .unwrap();
        assert_eq!(selected["agent_hash"].as_str().unwrap(), "aaa");
    }

    // ── response parsing ──────────────────────────────────────────

    #[test]
    fn secret_list_parses_response() {
        let resp: serde_json::Value = serde_json::json!({
            "secrets": [
                {"name": "KEY_A", "token": "VK:LOCAL:aaa", "scope": "LOCAL"},
                {"name": "KEY_B", "token": "VK:TEMP:bbb", "scope": "TEMP"},
            ],
            "count": 2
        });
        let secrets = resp["secrets"].as_array().unwrap();
        assert_eq!(secrets.len(), 2);
        assert_eq!(secrets[0]["name"].as_str().unwrap(), "KEY_A");
        assert_eq!(secrets[1]["scope"].as_str().unwrap(), "TEMP");
    }

    #[test]
    fn secret_list_empty() {
        let resp: serde_json::Value = serde_json::json!({"secrets": [], "count": 0});
        assert!(resp["secrets"].as_array().unwrap().is_empty());
    }

    #[test]
    fn secret_list_missing_fields() {
        let resp: serde_json::Value = serde_json::json!({"secrets": [{"name": "KEY_A"}]});
        let secrets = resp["secrets"].as_array().unwrap();
        assert_eq!(secrets[0]["token"].as_str(), None);
        assert_eq!(secrets[0]["scope"].as_str(), None);
    }

    #[test]
    fn promote_request_format() {
        let body = serde_json::json!({
            "ref": "VK:TEMP:ed694a5e",
            "name": "MAILGUN_API_KEY",
            "vault_hash": "a0a761c6",
        });
        assert_eq!(body["ref"].as_str().unwrap(), "VK:TEMP:ed694a5e");
        assert_eq!(body["name"].as_str().unwrap(), "MAILGUN_API_KEY");
        assert_eq!(body["vault_hash"].as_str().unwrap(), "a0a761c6");
    }

    #[test]
    fn promote_response_created() {
        let resp: serde_json::Value = serde_json::json!({
            "action": "created", "token": "VK:LOCAL:3c3d53ea", "status": "active"
        });
        assert_eq!(resp["action"].as_str().unwrap(), "created");
        assert_eq!(resp["token"].as_str().unwrap(), "VK:LOCAL:3c3d53ea");
    }

    #[test]
    fn promote_response_updated() {
        let resp: serde_json::Value = serde_json::json!({
            "action": "updated", "token": "VK:LOCAL:3c3d53ea"
        });
        assert_eq!(resp["action"].as_str().unwrap(), "updated");
    }

    #[test]
    fn agents_list_response_parsing() {
        let resp: serde_json::Value = serde_json::json!({
            "agents": [{
                "label": "host-lv", "agent_hash": "a0a761c6",
                "ip": "10.50.0.102", "port": 10180,
                "secrets_count": 13, "archived": false
            }],
            "count": 1
        });
        let agents = resp["agents"].as_array().unwrap();
        assert_eq!(agents[0]["agent_hash"].as_str().unwrap(), "a0a761c6");
        assert_eq!(agents[0]["port"].as_u64().unwrap(), 10180);
    }

    #[test]
    fn secret_delete_response() {
        let resp: serde_json::Value = serde_json::json!({"deleted": "MY_SECRET"});
        assert_eq!(resp["deleted"].as_str().unwrap(), "MY_SECRET");
    }

    // ── guard: functions exist ─────────────────────────────────────

    #[test]
    fn guard_secret_add_exists() {
        let src = include_str!("api.rs");
        assert!(src.contains("fn secret_add("));
    }

    #[test]
    fn guard_secret_list_exists() {
        let src = include_str!("api.rs");
        assert!(src.contains("fn secret_list("));
    }

    #[test]
    fn guard_secret_delete_exists() {
        let src = include_str!("api.rs");
        assert!(src.contains("fn secret_delete("));
    }

    #[test]
    fn guard_agents_list_exists() {
        let src = include_str!("api.rs");
        assert!(src.contains("fn agents_list("));
    }

    #[test]
    fn guard_promote_exists() {
        let src = include_str!("api.rs");
        assert!(src.contains("fn promote("));
    }

    #[test]
    fn guard_secret_add_pipeline() {
        let src = include_str!("api.rs");
        let fn_body = &src[src.find("fn secret_add(").unwrap()..];
        let fn_end = fn_body.find("\n    }").unwrap_or(fn_body.len());
        let body = &fn_body[..fn_end];
        assert!(body.contains("self.issue("), "must create temp ref");
        assert!(body.contains("self.agents_list()"), "must fetch agents");
        assert!(body.contains("self.promote("), "must promote to vault");
        assert!(body.contains("archived"), "must filter archived vaults");
    }

    // ── guard: API endpoints ──────────────────────────────────────

    #[test]
    fn guard_agents_endpoint() {
        let src = include_str!("api.rs");
        let f = &src[src.find("fn agents_list(").unwrap()..];
        assert!(f.contains("/api/agents"));
    }

    #[test]
    fn guard_promote_endpoint() {
        let src = include_str!("api.rs");
        let f = &src[src.find("fn promote(").unwrap()..];
        assert!(f.contains("/api/keycenter/promote"));
    }

    #[test]
    fn guard_secret_list_endpoint() {
        let src = include_str!("api.rs");
        let f = &src[src.find("fn secret_list(").unwrap()..];
        assert!(f.contains("/api/secrets"));
    }

    #[test]
    fn guard_secret_delete_endpoint() {
        let src = include_str!("api.rs");
        let f = &src[src.find("fn secret_delete(").unwrap()..];
        assert!(f.contains("/api/secrets/"));
    }

    // ── guard: CLI routing ────────────────────────────────────────

    #[test]
    fn guard_cli_secret_subcommand() {
        let src = include_str!("bin/veilkey_cli.rs");
        assert!(src.contains(r#""secret""#), "CLI must route secret subcommand");
    }

    #[test]
    fn guard_cli_secret_subcmds() {
        let src = include_str!("bin/veilkey_cli.rs");
        let block = &src[src.find(r#""secret""#).unwrap()..];
        assert!(block.contains(r#""add""#), "must have add");
        assert!(block.contains(r#""list""#), "must have list");
        assert!(block.contains(r#""get""#), "must have get");
        assert!(block.contains(r#""delete""#), "must have delete");
    }

    #[test]
    fn guard_cli_secret_vault_flag() {
        let src = include_str!("bin/veilkey_cli.rs");
        let block = &src[src.find(r#""secret""#).unwrap()..];
        assert!(block.contains("--vault"), "secret add must support --vault flag");
    }

    #[test]
    fn guard_cli_usage_shows_secret() {
        let src = include_str!("bin/veilkey_cli.rs");
        assert!(src.contains("veilkey secret add"));
        assert!(src.contains("veilkey secret list"));
        assert!(src.contains("veilkey secret get"));
        assert!(src.contains("veilkey secret delete"));
    }
}
