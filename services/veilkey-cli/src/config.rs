use regex::{Regex, RegexBuilder};
use serde::Deserialize;
use std::fs;

const EMBEDDED_PATTERNS: &str = include_str!("../patterns.yml");

#[derive(Debug, Deserialize)]
pub struct PatternDef {
    pub name: String,
    pub regex: String,
    #[serde(default = "default_confidence")]
    pub confidence: i32,
    #[serde(default)]
    pub group: usize,
}

pub const DEFAULT_PATTERN_CONFIDENCE: i32 = 70;
pub const DEFAULT_REGEX_SIZE_LIMIT: usize = 64 * 1024 * 1024; // 64MB

fn regex_size_limit() -> usize {
    std::env::var("VEILKEY_REGEX_SIZE_LIMIT")
        .ok()
        .and_then(|v| v.parse::<usize>().ok())
        .unwrap_or(DEFAULT_REGEX_SIZE_LIMIT)
}
pub const DEFAULT_ENTROPY_MIN_LENGTH: usize = 16;
pub const DEFAULT_ENTROPY_THRESHOLD: f64 = 3.5;
pub const DEFAULT_ENTROPY_CONFIDENCE_BOOST: i32 = 20;
pub const DEFAULT_SENSITIVE_BOOST: i32 = 15;

fn default_confidence() -> i32 {
    DEFAULT_PATTERN_CONFIDENCE
}

#[derive(Debug, Deserialize, Default)]
pub struct EntropyConfig {
    #[serde(default = "default_min_length")]
    pub min_length: usize,
    #[serde(default = "default_threshold")]
    pub threshold: f64,
    #[serde(default = "default_confidence_boost")]
    pub confidence_boost: i32,
}

fn default_min_length() -> usize {
    DEFAULT_ENTROPY_MIN_LENGTH
}
fn default_threshold() -> f64 {
    DEFAULT_ENTROPY_THRESHOLD
}
fn default_confidence_boost() -> i32 {
    DEFAULT_ENTROPY_CONFIDENCE_BOOST
}

#[derive(Debug, Deserialize, Default)]
pub struct SensitiveContext {
    #[serde(default)]
    pub keywords: Vec<String>,
    #[serde(default = "default_sensitive_boost")]
    pub confidence_boost: i32,
}

fn default_sensitive_boost() -> i32 {
    DEFAULT_SENSITIVE_BOOST
}

#[derive(Debug, Deserialize)]
struct RawConfig {
    #[serde(default)]
    patterns: Vec<PatternDef>,
    #[serde(default)]
    entropy: EntropyConfig,
    #[serde(default)]
    excludes: Vec<String>,
    #[serde(default)]
    sensitive_context: SensitiveContext,
}

pub struct CompiledPattern {
    pub name: String,
    pub regex: Regex,
    pub confidence: i32,
    pub group: usize,
}

pub struct CompiledConfig {
    pub patterns: Vec<CompiledPattern>,
    pub entropy: EntropyConfig,
    pub excludes: Vec<Regex>,
    pub sensitive_keywords: Vec<String>,
    pub sensitive_boost: i32,
}

pub fn load_config(path: Option<&str>) -> Result<CompiledConfig, String> {
    let data = match path {
        Some(p) => {
            fs::read_to_string(p).map_err(|e| format!("cannot read patterns file: {}", e))?
        }
        None => EMBEDDED_PATTERNS.to_string(),
    };

    let raw: RawConfig =
        serde_yaml::from_str(&data).map_err(|e| format!("cannot parse patterns: {}", e))?;

    let mut compiled = CompiledConfig {
        patterns: Vec::new(),
        entropy: raw.entropy,
        excludes: Vec::new(),
        sensitive_keywords: raw.sensitive_context.keywords,
        sensitive_boost: raw.sensitive_context.confidence_boost,
    };

    if compiled.entropy.min_length == 0 {
        compiled.entropy.min_length = DEFAULT_ENTROPY_MIN_LENGTH;
    }
    if compiled.entropy.threshold == 0.0 {
        compiled.entropy.threshold = DEFAULT_ENTROPY_THRESHOLD;
    }
    if compiled.entropy.confidence_boost == 0 {
        compiled.entropy.confidence_boost = DEFAULT_ENTROPY_CONFIDENCE_BOOST;
    }
    if compiled.sensitive_boost == 0 {
        compiled.sensitive_boost = DEFAULT_SENSITIVE_BOOST;
    }

    let size_limit = regex_size_limit();
    for p in raw.patterns {
        match RegexBuilder::new(&p.regex).size_limit(size_limit).build() {
            Ok(re) => compiled.patterns.push(CompiledPattern {
                name: p.name,
                regex: re,
                confidence: p.confidence,
                group: p.group,
            }),
            Err(e) => eprintln!("WARNING: invalid regex for {}: {}", p.name, e),
        }
    }

    for ex in raw.excludes {
        if let Ok(re) = RegexBuilder::new(&ex).size_limit(size_limit).build() {
            compiled.excludes.push(re);
        }
    }

    Ok(compiled)
}
