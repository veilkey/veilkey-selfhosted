use std::env;
use std::fs;
use std::path::PathBuf;

pub fn default_state_dir() -> PathBuf {
    let tmp = env::var("TMPDIR").unwrap_or_else(|_| std::env::temp_dir().to_string_lossy().into_owned());
    PathBuf::from(tmp).join("veilkey-cli")
}

pub fn state_dir() -> PathBuf {
    match env::var("VEILKEY_STATE_DIR") {
        Ok(v) if !v.is_empty() => PathBuf::from(v),
        _ => default_state_dir(),
    }
}

pub fn paste_mode_path() -> PathBuf {
    state_dir().join("paste-mode")
}

pub fn current_paste_mode() -> &'static str {
    match fs::read_to_string(paste_mode_path()) {
        Ok(data) => {
            if data.trim().to_lowercase() == "off" { "off" } else { "on" }
        }
        Err(_) => "on",
    }
}

pub fn set_paste_mode(mode: &str) -> Result<(), String> {
    let mode = mode.trim().to_lowercase();
    if mode != "on" && mode != "off" {
        return Err(format!("invalid paste mode: {}", mode));
    }
    let dir = state_dir();
    fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    fs::write(paste_mode_path(), format!("{}\n", mode)).map_err(|e| e.to_string())
}
