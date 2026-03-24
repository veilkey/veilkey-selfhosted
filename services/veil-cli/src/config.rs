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

#[cfg(test)]
mod tests {
    use super::*;

    // ── Default config (embedded patterns.yml) ────────────────────

    #[test]
    fn test_load_default_config_succeeds() {
        let cfg = load_config(None).unwrap();
        assert!(!cfg.patterns.is_empty(), "embedded patterns must load");
    }

    #[test]
    fn test_default_entropy_values() {
        let cfg = load_config(None).unwrap();
        assert!(cfg.entropy.min_length > 0);
        assert!(cfg.entropy.threshold > 0.0);
        assert!(cfg.entropy.confidence_boost > 0);
    }

    #[test]
    fn test_default_sensitive_boost() {
        let cfg = load_config(None).unwrap();
        assert!(cfg.sensitive_boost > 0);
    }

    // ── Custom YAML config ────────────────────────────────────────

    #[test]
    fn test_load_custom_patterns() {
        let yaml = r#"
patterns:
  - name: test-pattern
    regex: 'TEST_[A-Z0-9]{8}'
    confidence: 80
    group: 0
entropy:
  min_length: 20
  threshold: 4.0
  confidence_boost: 25
excludes:
  - '^SAFE_'
sensitive_context:
  keywords:
    - password
    - secret
  confidence_boost: 10
"#;
        let tmp = std::env::temp_dir().join("veilkey_test_config.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();

        assert_eq!(cfg.patterns.len(), 1);
        assert_eq!(cfg.patterns[0].name, "test-pattern");
        assert_eq!(cfg.patterns[0].confidence, 80);
        assert_eq!(cfg.patterns[0].group, 0);
        assert!(cfg.patterns[0].regex.is_match("TEST_ABCD1234"));
        assert!(!cfg.patterns[0].regex.is_match("TEST_abc"));

        assert_eq!(cfg.entropy.min_length, 20);
        assert!((cfg.entropy.threshold - 4.0).abs() < f64::EPSILON);
        assert_eq!(cfg.entropy.confidence_boost, 25);

        assert_eq!(cfg.excludes.len(), 1);
        assert!(cfg.excludes[0].is_match("SAFE_VALUE"));

        assert_eq!(cfg.sensitive_keywords, vec!["password", "secret"]);
        assert_eq!(cfg.sensitive_boost, 10);

        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_load_empty_config() {
        let yaml = "patterns: []\n";
        let tmp = std::env::temp_dir().join("veilkey_test_empty.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();

        assert!(cfg.patterns.is_empty());
        // Defaults should be applied for zero values
        assert_eq!(cfg.entropy.min_length, DEFAULT_ENTROPY_MIN_LENGTH);
        assert!((cfg.entropy.threshold - DEFAULT_ENTROPY_THRESHOLD).abs() < f64::EPSILON);
        assert_eq!(cfg.entropy.confidence_boost, DEFAULT_ENTROPY_CONFIDENCE_BOOST);
        assert_eq!(cfg.sensitive_boost, DEFAULT_SENSITIVE_BOOST);

        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_default_confidence_value() {
        let yaml = r#"
patterns:
  - name: no-confidence
    regex: 'NOCONF_[A-Z]+'
"#;
        let tmp = std::env::temp_dir().join("veilkey_test_defconf.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();
        assert_eq!(cfg.patterns[0].confidence, DEFAULT_PATTERN_CONFIDENCE);
        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_invalid_regex_skipped_with_warning() {
        let yaml = r#"
patterns:
  - name: valid
    regex: 'VALID_[A-Z]+'
    confidence: 50
  - name: invalid
    regex: '(?P<bad'
    confidence: 50
"#;
        let tmp = std::env::temp_dir().join("veilkey_test_badre.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();
        // Invalid regex is skipped, valid one remains
        assert_eq!(cfg.patterns.len(), 1);
        assert_eq!(cfg.patterns[0].name, "valid");
        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_invalid_exclude_regex_skipped() {
        let yaml = r#"
patterns: []
excludes:
  - '^valid$'
  - '(?P<broken'
"#;
        let tmp = std::env::temp_dir().join("veilkey_test_badexcl.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();
        assert_eq!(cfg.excludes.len(), 1);
        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_load_nonexistent_file_returns_error() {
        let result = load_config(Some("/nonexistent/veilkey_test.yml"));
        assert!(result.is_err());
    }

    #[test]
    fn test_load_invalid_yaml_returns_error() {
        let tmp = std::env::temp_dir().join("veilkey_test_invalid.yml");
        std::fs::write(&tmp, "{{{{not yaml").unwrap();
        let result = load_config(Some(tmp.to_str().unwrap()));
        assert!(result.is_err());
        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_compiled_pattern_with_capture_group() {
        let yaml = r#"
patterns:
  - name: grouped
    regex: 'KEY=([A-Z0-9]{8})'
    confidence: 60
    group: 1
"#;
        let tmp = std::env::temp_dir().join("veilkey_test_group.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();
        assert_eq!(cfg.patterns[0].group, 1);

        let caps = cfg.patterns[0].regex.captures("KEY=ABCD1234").unwrap();
        assert_eq!(caps.get(1).unwrap().as_str(), "ABCD1234");

        let _ = std::fs::remove_file(&tmp);
    }

    #[test]
    fn test_multiple_patterns_all_compile() {
        let yaml = r#"
patterns:
  - name: pat1
    regex: 'AAA_[0-9]+'
  - name: pat2
    regex: 'BBB_[a-z]+'
  - name: pat3
    regex: 'CCC_[A-Za-z0-9]+'
"#;
        let tmp = std::env::temp_dir().join("veilkey_test_multi.yml");
        std::fs::write(&tmp, yaml).unwrap();
        let cfg = load_config(Some(tmp.to_str().unwrap())).unwrap();
        assert_eq!(cfg.patterns.len(), 3);
        let _ = std::fs::remove_file(&tmp);
    }
}
