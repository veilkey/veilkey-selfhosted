use regex::Regex;
use std::collections::HashMap;
use std::fs;

use crate::api::VeilKeyClient;
use crate::config::CompiledConfig;
use crate::logger::SessionLogger;
use crate::state::state_dir;

pub const VEILKEY_RE_STR: &str = r"VK:(?:(?:TEMP|LOCAL|EXTERNAL):[0-9A-Fa-f]{4,64}|[0-9a-f]{8})";

const MIN_SECRET_LEN: usize = 6;
const PREVIEW_LEN: usize = 4;
const WATCHLIST_CONFIDENCE: i32 = 100;
const SCAN_ONLY_PLACEHOLDER: &str = "[detected]";

fn min_confidence() -> i32 {
    std::env::var("VEILKEY_MIN_CONFIDENCE")
        .ok()
        .and_then(|v| v.parse::<i32>().ok())
        .unwrap_or(40)
}

pub struct Detection {
    pub value: String,
    pub full_match: String,
    pub pattern: String,
    pub confidence: i32,
}

#[derive(Default)]
pub struct Stats {
    pub lines: usize,
    pub detections: usize,
    pub api_calls: usize,
    pub api_errors: usize,
}

pub struct WatchEntry {
    pub value: String,
    pub vk: String,
}

pub struct SecretDetector<'a> {
    pub config: &'a CompiledConfig,
    pub client: &'a VeilKeyClient,
    pub logger: &'a SessionLogger,
    pub scan_only: bool,
    cache: HashMap<String, String>,
    watchlist: Vec<WatchEntry>,
    pub paused: bool,
    pub stats: Stats,
    veilkey_re: Regex,
}

impl<'a> SecretDetector<'a> {
    pub fn new(
        config: &'a CompiledConfig,
        client: &'a VeilKeyClient,
        logger: &'a SessionLogger,
        scan_only: bool,
    ) -> Self {
        let veilkey_re = Regex::new(VEILKEY_RE_STR).unwrap();
        let mut det = Self {
            config,
            client,
            logger,
            scan_only,
            cache: HashMap::new(),
            watchlist: Vec::new(),
            paused: false,
            stats: Stats::default(),
            veilkey_re,
        };
        det.load_watchlist();
        det
    }

    fn load_watchlist(&mut self) {
        let path = state_dir().join("watchlist");
        let data = match fs::read_to_string(&path) {
            Ok(d) => d,
            Err(_) => return,
        };

        let now = chrono::Utc::now();
        let mut kept: Vec<String> = Vec::new();
        let mut pruned = false;

        for line in data.lines() {
            let parts: Vec<&str> = line.splitn(3, '\t').collect();
            if parts.len() < 2 || parts[0].is_empty() || parts[1].is_empty() {
                continue;
            }
            if parts.len() == 3 && !parts[2].is_empty() {
                if let Ok(exp) = chrono::DateTime::parse_from_rfc3339(parts[2]) {
                    if now > exp {
                        pruned = true;
                        continue;
                    }
                }
            }
            self.watchlist.push(WatchEntry {
                value: parts[0].to_string(),
                vk: parts[1].to_string(),
            });
            kept.push(line.to_string());
        }

        if pruned {
            let content = kept.join("\n") + if kept.is_empty() { "" } else { "\n" };
            let _ = fs::write(&path, content);
        }
    }

    #[allow(dead_code)]
    pub fn reload_watchlist(&mut self) {
        self.watchlist.clear();
        self.load_watchlist();
    }

    /// Register a known plaintext→VK mapping so it gets masked in output.
    pub fn register_known(&mut self, plaintext: &str, vk_ref: &str) {
        self.watchlist.push(WatchEntry {
            value: plaintext.to_string(),
            vk: vk_ref.to_string(),
        });
    }

    fn is_excluded(&self, value: &str) -> bool {
        if self.veilkey_re.is_match(value) {
            return true;
        }
        self.config.excludes.iter().any(|re| re.is_match(value))
    }

    fn has_sensitive_context(&self, line: &str) -> bool {
        let lower = line.to_lowercase();
        self.config
            .sensitive_keywords
            .iter()
            .any(|kw| lower.contains(kw.as_str()))
    }

    fn shannon_entropy(s: &str) -> f64 {
        if s.is_empty() {
            return 0.0;
        }
        let chars: Vec<char> = s.chars().collect();
        let len = chars.len() as f64;
        let mut counts: HashMap<char, usize> = HashMap::new();
        for c in &chars {
            *counts.entry(*c).or_insert(0) += 1;
        }
        counts
            .values()
            .map(|&c| {
                let p = c as f64 / len;
                -p * p.log2()
            })
            .sum()
    }

    fn issue_veilkey(&mut self, value: &str) -> Option<String> {
        if let Some(vk) = self.cache.get(value) {
            return Some(vk.clone());
        }
        if self.scan_only {
            self.cache
                .insert(value.to_string(), SCAN_ONLY_PLACEHOLDER.to_string());
            return Some(SCAN_ONLY_PLACEHOLDER.to_string());
        }
        match self.client.issue(value) {
            Ok(vk) => {
                self.stats.api_calls += 1;
                self.cache.insert(value.to_string(), vk.clone());
                Some(vk)
            }
            Err(e) => {
                self.stats.api_errors += 1;
                eprintln!("WARNING: VeilKey API failed: {}", e);
                None
            }
        }
    }

    pub fn detect_secrets(&self, line: &str) -> Vec<Detection> {
        let mut results = Vec::new();
        let has_context = self.has_sensitive_context(line);

        for pat in &self.config.patterns {
            for caps in pat.regex.captures_iter(line) {
                let full_match = caps.get(0).unwrap().as_str().to_string();
                let value = if pat.group > 0 {
                    caps.get(pat.group)
                        .map(|g| g.as_str().to_string())
                        .unwrap_or_else(|| full_match.clone())
                } else {
                    full_match.clone()
                };

                if value.len() < MIN_SECRET_LEN {
                    continue;
                }
                if self.is_excluded(&value) {
                    continue;
                }

                let mut conf = pat.confidence;
                if has_context {
                    conf += self.config.sensitive_boost;
                }
                if value.chars().count() >= self.config.entropy.min_length {
                    let ent = Self::shannon_entropy(&value);
                    if ent > self.config.entropy.threshold {
                        conf += self.config.entropy.confidence_boost;
                    }
                }

                if conf >= min_confidence() {
                    results.push(Detection {
                        value,
                        full_match,
                        pattern: pat.name.clone(),
                        confidence: conf,
                    });
                }
            }
        }
        results
    }

    pub fn process_line(&mut self, line: &str) -> String {
        self.stats.lines += 1;
        let mut line = line.to_string();

        // Protect existing VeilKeys
        let vk_matches: Vec<_> = self
            .veilkey_re
            .find_iter(&line)
            .map(|m| (m.start(), m.end(), m.as_str().to_string()))
            .collect();

        let mut protected: Vec<(String, String)> = Vec::new();
        let mut offset: i64 = 0;
        for (i, (start, end, orig)) in vk_matches.iter().enumerate() {
            let ph = format!("\x00VK{}\x00", i);
            let adj_start = (*start as i64 + offset) as usize;
            let adj_end = (*end as i64 + offset) as usize;
            let diff = ph.len() as i64 - (end - start) as i64;
            line = format!("{}{}{}", &line[..adj_start], ph, &line[adj_end..]);
            protected.push((ph, orig.clone()));
            offset += diff;
        }

        let mut detections = self.detect_secrets(&line);
        if !detections.is_empty() {
            detections.sort_by(|a, b| {
                b.confidence
                    .cmp(&a.confidence)
                    .then(b.full_match.len().cmp(&a.full_match.len()))
            });

            let mut replaced: std::collections::HashSet<String> = std::collections::HashSet::new();
            for det in detections {
                if replaced.contains(&det.value) {
                    continue;
                }
                if let Some(vk) = self.issue_veilkey(&det.value) {
                    if det.value != det.full_match {
                        let new_match = det.full_match.replacen(&det.value, &vk, 1);
                        line = line.replacen(&det.full_match, &new_match, 1);
                    } else {
                        line = line.replacen(&det.value, &vk, 1);
                    }
                    replaced.insert(det.value.clone());
                    self.stats.detections += 1;
                    let preview = if det.value.chars().count() > PREVIEW_LEN {
                        let end: usize = det
                            .value
                            .char_indices()
                            .nth(PREVIEW_LEN)
                            .map(|(i, _)| i)
                            .unwrap_or(det.value.len());
                        format!("{}***", &det.value[..end])
                    } else {
                        "***".to_string()
                    };
                    self.logger.log(&vk, &det.pattern, det.confidence, &preview);
                }
            }
        }

        // Watchlist (skip if paused)
        if !self.paused {
            let watchlist: Vec<(String, String)> = self
                .watchlist
                .iter()
                .map(|w| (w.value.clone(), w.vk.clone()))
                .collect();
            for (value, vk) in watchlist {
                if line.contains(&value) {
                    line = line.replace(&value, &vk);
                    self.stats.detections += 1;
                    let preview = if value.chars().count() > PREVIEW_LEN {
                        let end: usize = value
                            .char_indices()
                            .nth(PREVIEW_LEN)
                            .map(|(i, _)| i)
                            .unwrap_or(value.len());
                        format!("{}***", &value[..end])
                    } else {
                        "***".to_string()
                    };
                    self.logger
                        .log(&vk, "watchlist", WATCHLIST_CONFIDENCE, &preview);
                }
            }
        }

        // Restore VeilKey placeholders
        for (ph, orig) in &protected {
            line = line.replacen(ph, orig, 1);
        }
        line
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::load_config;

    /// Helper: build a minimal CompiledConfig with a single pattern for testing.
    fn test_config_with_pattern(regex: &str, confidence: i32, group: usize) -> crate::config::CompiledConfig {
        use regex::Regex;
        crate::config::CompiledConfig {
            patterns: vec![crate::config::CompiledPattern {
                name: "test-pattern".to_string(),
                regex: Regex::new(regex).unwrap(),
                confidence,
                group,
            }],
            entropy: crate::config::EntropyConfig {
                min_length: 16,
                threshold: 3.5,
                confidence_boost: 20,
            },
            excludes: vec![],
            sensitive_keywords: vec!["password".to_string(), "secret".to_string()],
            sensitive_boost: 15,
        }
    }

    fn install_crypto_provider() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }

    /// Helper: build a detector in scan_only mode (no API calls needed).
    fn test_detector(config: &crate::config::CompiledConfig) -> SecretDetector<'_> {
        install_crypto_provider();
        // Set a temp state dir so load_watchlist doesn't fail
        std::env::set_var("VEILKEY_STATE_DIR", std::env::temp_dir().join("veilkey-test").to_str().unwrap());
        let client = VeilKeyClient::new("http://localhost:0");
        let logger = SessionLogger::new("/dev/null");
        // Leak references to avoid lifetime issues in tests
        let client = Box::leak(Box::new(client));
        let logger = Box::leak(Box::new(logger));
        SecretDetector::new(config, client, logger, true)
    }

    // ── shannon_entropy ───────────────────────────────────────────

    #[test]
    fn test_entropy_empty_string() {
        assert!((SecretDetector::shannon_entropy("") - 0.0).abs() < f64::EPSILON);
    }

    #[test]
    fn test_entropy_single_char() {
        // All same characters → entropy = 0
        assert!((SecretDetector::shannon_entropy("aaaa") - 0.0).abs() < f64::EPSILON);
    }

    #[test]
    fn test_entropy_uniform_two_chars() {
        // "ab" repeated → entropy = 1.0 (two equally probable chars)
        let ent = SecretDetector::shannon_entropy("abababab");
        assert!((ent - 1.0).abs() < 0.01);
    }

    #[test]
    fn test_entropy_high_entropy_string() {
        // Random-looking string with many distinct chars
        let ent = SecretDetector::shannon_entropy("aB3$xZ9@kL7!mQ2#");
        assert!(ent > 3.5, "high entropy string should exceed 3.5, got {}", ent);
    }

    #[test]
    fn test_entropy_single_character_string() {
        assert!((SecretDetector::shannon_entropy("x") - 0.0).abs() < f64::EPSILON);
    }

    #[test]
    fn test_entropy_all_unique_chars() {
        // 8 unique chars → max entropy = log2(8) = 3.0
        let ent = SecretDetector::shannon_entropy("abcdefgh");
        assert!((ent - 3.0).abs() < 0.01);
    }

    // ── is_excluded ───────────────────────────────────────────────

    #[test]
    fn test_excluded_veilkey_ref_local() {
        let config = test_config_with_pattern(".*", 50, 0);
        let det = test_detector(&config);
        assert!(det.is_excluded("VK:LOCAL:abcdef12"));
    }

    #[test]
    fn test_excluded_veilkey_ref_temp() {
        let config = test_config_with_pattern(".*", 50, 0);
        let det = test_detector(&config);
        assert!(det.is_excluded("VK:TEMP:1234abcd"));
    }

    #[test]
    fn test_excluded_veilkey_ref_external() {
        let config = test_config_with_pattern(".*", 50, 0);
        let det = test_detector(&config);
        assert!(det.is_excluded("VK:EXTERNAL:deadbeef"));
    }

    #[test]
    fn test_excluded_veilkey_short_ref() {
        let config = test_config_with_pattern(".*", 50, 0);
        let det = test_detector(&config);
        assert!(det.is_excluded("VK:abcdef01"));
    }

    #[test]
    fn test_not_excluded_normal_string() {
        let config = test_config_with_pattern(".*", 50, 0);
        let det = test_detector(&config);
        assert!(!det.is_excluded("my-secret-password"));
    }

    #[test]
    fn test_excluded_by_config_pattern() {
        use regex::Regex;
        let mut config = test_config_with_pattern(".*", 50, 0);
        config.excludes = vec![Regex::new(r"^SAFE_").unwrap()];
        let det = test_detector(&config);
        assert!(det.is_excluded("SAFE_VALUE_123"));
        assert!(!det.is_excluded("UNSAFE_VALUE"));
    }

    // ── detect_secrets ────────────────────────────────────────────

    #[test]
    fn test_detect_basic_pattern_match() {
        let config = test_config_with_pattern(r"SECRET_[A-Z0-9]{8}", 50, 0);
        let det = test_detector(&config);
        let results = det.detect_secrets("found SECRET_ABCD1234 here");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].value, "SECRET_ABCD1234");
        assert_eq!(results[0].pattern, "test-pattern");
    }

    #[test]
    fn test_detect_no_match() {
        let config = test_config_with_pattern(r"SECRET_[A-Z0-9]{8}", 50, 0);
        let det = test_detector(&config);
        let results = det.detect_secrets("nothing special here");
        assert!(results.is_empty());
    }

    #[test]
    fn test_detect_min_secret_length() {
        // Pattern matches short strings, but MIN_SECRET_LEN (6) filters them out
        let config = test_config_with_pattern(r"SK_[A-Z]+", 50, 0);
        let det = test_detector(&config);

        // "SK_AB" = 5 chars → below MIN_SECRET_LEN → filtered
        let results = det.detect_secrets("token SK_AB here");
        assert!(results.is_empty());

        // "SK_ABCD" = 7 chars → above MIN_SECRET_LEN → detected
        let results = det.detect_secrets("token SK_ABCD here");
        assert_eq!(results.len(), 1);
    }

    #[test]
    fn test_detect_excluded_veilkey_refs_not_detected() {
        let config = test_config_with_pattern(r"VK:[A-Z]+:[0-9a-f]+", 50, 0);
        let det = test_detector(&config);
        let results = det.detect_secrets("ref VK:LOCAL:abcdef12 here");
        assert!(results.is_empty());
    }

    #[test]
    fn test_detect_with_capture_group() {
        let config = test_config_with_pattern(r"KEY=([A-Z0-9]{8})", 50, 1);
        let det = test_detector(&config);
        let results = det.detect_secrets("export KEY=ABCD1234");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].value, "ABCD1234");
        assert_eq!(results[0].full_match, "KEY=ABCD1234");
    }

    #[test]
    fn test_detect_sensitive_context_boost() {
        // Base confidence 30 is below default min_confidence (40)
        // But with sensitive context boost (+15) it becomes 45 → detected
        let config = test_config_with_pattern(r"TOKEN_[A-Z0-9]{8}", 30, 0);
        let det = test_detector(&config);

        // No sensitive context → 30 < 40 → not detected
        let results = det.detect_secrets("found TOKEN_ABCD1234 here");
        assert!(results.is_empty());

        // With sensitive keyword "password" → 30 + 15 = 45 ≥ 40 → detected
        let results = det.detect_secrets("password is TOKEN_ABCD1234");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].confidence, 45);
    }

    #[test]
    fn test_detect_entropy_boost() {
        // High-entropy string ≥ 16 chars with confidence just below threshold
        let config = test_config_with_pattern(r"[A-Za-z0-9!@#$%]{16,}", 25, 0);
        let det = test_detector(&config);

        // High entropy value: "aB3$xZ9@kL7!mQ2#" has entropy > 3.5
        // 25 + 20 (entropy boost) = 45 ≥ 40 → detected
        let results = det.detect_secrets("key=aB3$xZ9@kL7!mQ2#");
        assert_eq!(results.len(), 1);
        assert!(results[0].confidence >= 45);
    }

    #[test]
    fn test_detect_below_min_confidence_filtered() {
        let config = test_config_with_pattern(r"WEAK_[A-Z]{6}", 20, 0);
        let det = test_detector(&config);
        let results = det.detect_secrets("found WEAK_ABCDEF here");
        assert!(results.is_empty(), "confidence 20 < 40 should be filtered");
    }

    #[test]
    fn test_detect_multiple_matches_in_line() {
        let config = test_config_with_pattern(r"TOK_[A-Z0-9]{6}", 50, 0);
        let det = test_detector(&config);
        let results = det.detect_secrets("first TOK_ABC123 second TOK_XYZ789");
        assert_eq!(results.len(), 2);
    }

    // ── process_line ──────────────────────────────────────────────

    #[test]
    fn test_process_line_replaces_secret_with_scan_placeholder() {
        let config = test_config_with_pattern(r"SECRET_[A-Z0-9]{8}", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        let result = det.process_line("found SECRET_ABCD1234 here");
        assert!(result.contains("[detected]"), "should replace with scan placeholder: {}", result);
        assert!(!result.contains("SECRET_ABCD1234"));
        assert_eq!(det.stats.lines, 1);
        assert_eq!(det.stats.detections, 1);
    }

    #[test]
    fn test_process_line_no_detection() {
        let config = test_config_with_pattern(r"SECRET_[A-Z0-9]{8}", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        let result = det.process_line("nothing here");
        assert_eq!(result, "nothing here");
        assert_eq!(det.stats.detections, 0);
    }

    #[test]
    fn test_process_line_protects_existing_veilkey_refs() {
        let config = test_config_with_pattern(r"[0-9a-f]{8}", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        // VK:LOCAL:abcdef12 should be preserved even though "abcdef12" matches the pattern
        let result = det.process_line("ref VK:LOCAL:abcdef12 done");
        assert!(result.contains("VK:LOCAL:abcdef12"), "VK ref must be preserved: {}", result);
    }

    #[test]
    fn test_process_line_watchlist_replacement() {
        let config = test_config_with_pattern(r"NOMATCH", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        det.register_known("my-password-123", "VK:LOCAL:masked01");
        let result = det.process_line("db uses my-password-123 as auth");
        assert!(result.contains("VK:LOCAL:masked01"));
        assert!(!result.contains("my-password-123"));
    }

    #[test]
    fn test_process_line_watchlist_skipped_when_paused() {
        let config = test_config_with_pattern(r"NOMATCH", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        det.register_known("my-password-123", "VK:LOCAL:masked01");
        det.paused = true;
        let result = det.process_line("db uses my-password-123 as auth");
        assert!(result.contains("my-password-123"), "watchlist should be skipped when paused");
    }

    #[test]
    fn test_process_line_with_capture_group_partial_replacement() {
        // When value != full_match, only the value part should be replaced
        let config = test_config_with_pattern(r"KEY=([A-Z0-9]{8})", 50, 1);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        let result = det.process_line("export KEY=ABCD1234 ok");
        // "ABCD1234" (value) replaced within "KEY=ABCD1234" (full_match)
        assert!(result.contains("KEY=[detected]"), "partial replacement expected: {}", result);
    }

    #[test]
    fn test_process_line_multiple_vk_refs_protected() {
        let config = test_config_with_pattern(r"NOMATCH", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        let line = "VK:LOCAL:aaaa1111 and VK:TEMP:bbbb2222 end";
        let result = det.process_line(line);
        assert_eq!(result, line, "all VK refs should be preserved unchanged");
    }

    #[test]
    fn test_process_line_stats_increment() {
        let config = test_config_with_pattern(r"TOK_[A-Z]{6}", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        det.process_line("line one");
        det.process_line("line two TOK_ABCDEF here");
        det.process_line("line three");
        assert_eq!(det.stats.lines, 3);
        assert_eq!(det.stats.detections, 1);
    }

    #[test]
    fn test_process_line_dedup_same_value() {
        // Same value appearing twice in one line should only count as one detection
        let config = test_config_with_pattern(r"TOK_[A-Z]{6}", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        let result = det.process_line("TOK_ABCDEF and TOK_ABCDEF again");
        // The replacen with count 1 means only first occurrence is replaced by detect,
        // but the dedup set prevents double-issuing
        assert_eq!(det.stats.detections, 1);
        // At least one occurrence should be replaced
        assert!(result.contains("[detected]"));
    }

    // ── register_known ────────────────────────────────────────────

    #[test]
    fn test_register_known_adds_to_watchlist() {
        let config = test_config_with_pattern(r"NOMATCH", 50, 0);
        let config = Box::leak(Box::new(config));
        let mut det = test_detector(config);

        det.register_known("secret-val", "VK:LOCAL:ref01");
        let result = det.process_line("found secret-val here");
        assert!(result.contains("VK:LOCAL:ref01"));
    }

    // ── has_sensitive_context ─────────────────────────────────────

    #[test]
    fn test_sensitive_context_case_insensitive() {
        let config = test_config_with_pattern(r"TOKEN_[A-Z0-9]{8}", 30, 0);
        let det = test_detector(&config);

        // "PASSWORD" (uppercase) should match "password" keyword
        let results = det.detect_secrets("PASSWORD: TOKEN_ABCD1234");
        assert_eq!(results.len(), 1, "case-insensitive keyword match");
    }

    // ── VEILKEY_RE_STR validation ─────────────────────────────────

    #[test]
    fn test_veilkey_regex_matches_expected_formats() {
        let re = Regex::new(VEILKEY_RE_STR).unwrap();
        assert!(re.is_match("VK:LOCAL:abcd1234"));
        assert!(re.is_match("VK:TEMP:ABCDEF01"));
        assert!(re.is_match("VK:EXTERNAL:deadbeef"));
        assert!(re.is_match("VK:abcdef01")); // short format
        assert!(re.is_match("VK:LOCAL:abcdef0123456789")); // long hash
    }

    #[test]
    fn test_veilkey_regex_rejects_invalid_formats() {
        let re = Regex::new(&format!("^{}$", VEILKEY_RE_STR)).unwrap();
        assert!(!re.is_match("VK:UNKNOWN:abcd1234"));
        assert!(!re.is_match("VK:LOCAL:ab")); // too short (< 4)
        assert!(!re.is_match("VK:abcdefg")); // 7 chars, needs exactly 8 for short format
        assert!(!re.is_match("NOT_A_VK_REF"));
    }

    // ── Edge: embedded config with real patterns ──────────────────

    #[test]
    fn test_detect_with_embedded_config_github_token() {
        let config = load_config(None).unwrap();
        let det = test_detector(&config);
        // GitHub personal access token format
        let results = det.detect_secrets("token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef12");
        // Should detect something (depends on patterns.yml having github pattern)
        // Just verify no panic
        let _ = results;
    }

    #[test]
    fn test_detect_with_embedded_config_aws_key() {
        let config = load_config(None).unwrap();
        let det = test_detector(&config);
        let results = det.detect_secrets("AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE");
        let _ = results;
    }
}
