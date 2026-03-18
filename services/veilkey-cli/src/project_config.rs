use serde::Deserialize;
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Deserialize, Default)]
#[allow(dead_code)]
pub struct ProjectConfig {
    #[serde(rename = "patterns", default)]
    pub patterns_file: String,
    #[serde(default)]
    pub exclude_paths: Vec<String>,
    #[serde(default)]
    pub format: String,
    #[serde(default)]
    pub api_url: String,
    #[serde(default)]
    pub exit_code: bool,
}

/// Search order: explicit path → $VEILKEY_CONFIG → .veilkey.yml → .veilkey.yaml → ~/.config/veilkey/config.yml
/// Returns None if no config file is found or parse fails.
pub fn load_project_config(path: Option<&str>) -> Option<ProjectConfig> {
    if let Some(p) = path {
        return load_file(p);
    }

    if let Ok(p) = std::env::var("VEILKEY_CONFIG") {
        if !p.is_empty() {
            return load_file(&p);
        }
    }

    for name in &[".veilkey.yml", ".veilkey.yaml"] {
        if std::path::Path::new(name).exists() {
            return load_file(name);
        }
    }

    if let Ok(home) = std::env::var("HOME") {
        let user_cfg = PathBuf::from(home)
            .join(".config")
            .join("veilkey")
            .join("config.yml");
        if user_cfg.exists() {
            return load_file(&user_cfg.to_string_lossy());
        }
    }

    None
}

fn load_file(path: &str) -> Option<ProjectConfig> {
    let data = fs::read_to_string(path).ok()?;
    serde_yaml::from_str(&data).ok()
}
