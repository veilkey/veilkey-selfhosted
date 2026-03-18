use regex::Regex;
use std::collections::HashMap;
use std::fs;


use crate::api::VeilKeyClient;
use crate::config::CompiledConfig;
use crate::logger::SessionLogger;
use crate::state::state_dir;

pub const VEILKEY_RE_STR: &str =
    r"VK:(?:(?:TEMP|LOCAL|EXTERNAL):[0-9A-Fa-f]{4,64}|[0-9a-f]{8})";

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

    fn is_excluded(&self, value: &str) -> bool {
        if self.veilkey_re.is_match(value) {
            return true;
        }
        self.config.excludes.iter().any(|re| re.is_match(value))
    }

    fn has_sensitive_context(&self, line: &str) -> bool {
        let lower = line.to_lowercase();
        self.config.sensitive_keywords.iter().any(|kw| lower.contains(kw.as_str()))
    }

    fn shannon_entropy(s: &str) -> f64 {
        if s.is_empty() { return 0.0; }
        let chars: Vec<char> = s.chars().collect();
        let len = chars.len() as f64;
        let mut counts: HashMap<char, usize> = HashMap::new();
        for c in &chars { *counts.entry(*c).or_insert(0) += 1; }
        counts.values().map(|&c| {
            let p = c as f64 / len;
            -p * p.log2()
        }).sum()
    }

    fn issue_veilkey(&mut self, value: &str) -> Option<String> {
        if let Some(vk) = self.cache.get(value) {
            return Some(vk.clone());
        }
        if self.scan_only {
            self.cache.insert(value.to_string(), SCAN_ONLY_PLACEHOLDER.to_string());
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
            for m in pat.regex.find_iter(line) {
                let full_match = m.as_str().to_string();
                let value = if pat.group > 0 {
                    pat.regex.captures(m.as_str())
                        .and_then(|c| c.get(pat.group))
                        .map(|g| g.as_str().to_string())
                        .unwrap_or_else(|| full_match.clone())
                } else {
                    full_match.clone()
                };

                if value.len() < MIN_SECRET_LEN { continue; }
                if self.is_excluded(&value) { continue; }

                let mut conf = pat.confidence;
                if has_context { conf += self.config.sensitive_boost; }
                if value.chars().count() >= self.config.entropy.min_length {
                    let ent = Self::shannon_entropy(&value);
                    if ent > self.config.entropy.threshold {
                        conf += self.config.entropy.confidence_boost;
                    }
                }

                if conf >= min_confidence() {
                    results.push(Detection { value, full_match, pattern: pat.name.clone(), confidence: conf });
                }
            }
        }
        results
    }

    pub fn process_line(&mut self, line: &str) -> String {
        self.stats.lines += 1;
        let mut line = line.to_string();

        // Protect existing VeilKeys
        let vk_matches: Vec<_> = self.veilkey_re.find_iter(&line)
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
            detections.sort_by(|a, b| b.confidence.cmp(&a.confidence)
                .then(b.full_match.len().cmp(&a.full_match.len())));

            let mut replaced: HashMap<String, bool> = HashMap::new();
            for det in detections {
                if replaced.contains_key(&det.value) { continue; }
                if let Some(vk) = self.issue_veilkey(&det.value) {
                    if det.value != det.full_match {
                        let new_match = det.full_match.replacen(&det.value, &vk, 1);
                        line = line.replacen(&det.full_match, &new_match, 1);
                    } else {
                        line = line.replacen(&det.value, &vk, 1);
                    }
                    replaced.insert(det.value.clone(), true);
                    self.stats.detections += 1;
                    let preview = if det.value.len() > PREVIEW_LEN {
                        format!("{}***", &det.value[..PREVIEW_LEN])
                    } else { "***".to_string() };
                    self.logger.log(&vk, &det.pattern, det.confidence, &preview);
                }
            }
        }

        // Watchlist (skip if paused)
        if !self.paused {
            let watchlist: Vec<(String, String)> = self.watchlist.iter()
                .map(|w| (w.value.clone(), w.vk.clone()))
                .collect();
            for (value, vk) in watchlist {
                if line.contains(&value) {
                    line = line.replace(&value, &vk);
                    self.stats.detections += 1;
                    let preview = if value.len() > PREVIEW_LEN {
                        format!("{}***", &value[..PREVIEW_LEN])
                    } else { "***".to_string() };
                    self.logger.log(&vk, "watchlist", WATCHLIST_CONFIDENCE, &preview);
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
