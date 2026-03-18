use serde::{Deserialize, Serialize};
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::Path;

#[derive(Debug, Serialize, Deserialize)]
pub struct LogEntry {
    pub timestamp: String,
    pub veilkey: String,
    pub pattern: String,
    pub confidence: i32,
    pub preview: String,
}

pub struct SessionLogger {
    path: String,
}

impl SessionLogger {
    pub fn new(path: &str) -> Self {
        if let Some(dir) = Path::new(path).parent() {
            let _ = fs::create_dir_all(dir);
        }
        Self {
            path: path.to_string(),
        }
    }

    pub fn log(&self, veilkey: &str, pattern: &str, confidence: i32, preview: &str) {
        let entry = LogEntry {
            timestamp: chrono::Local::now().format("%Y-%m-%d %H:%M:%S").to_string(),
            veilkey: veilkey.to_string(),
            pattern: pattern.to_string(),
            confidence,
            preview: preview.to_string(),
        };

        if let Ok(data) = serde_json::to_string(&entry) {
            if let Ok(mut f) = OpenOptions::new()
                .append(true)
                .create(true)
                .open(&self.path)
            {
                let _ = writeln!(f, "{}", data);
            }
        }
    }

    pub fn read_entries(&self) -> Vec<LogEntry> {
        let data = match fs::read_to_string(&self.path) {
            Ok(d) => d,
            Err(_) => return Vec::new(),
        };

        data.lines()
            .filter(|l| !l.is_empty())
            .filter_map(|l| serde_json::from_str(l).ok())
            .collect()
    }

    pub fn count(&self) -> usize {
        self.read_entries().len()
    }

    pub fn clear(&self) -> std::io::Result<()> {
        fs::remove_file(&self.path)
    }
}
