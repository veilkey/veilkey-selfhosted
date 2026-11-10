use crate::state::open_private_file;
use serde::{Deserialize, Serialize};
use std::fs;
use std::io::Write;
use std::path::Path;

#[derive(Debug, Serialize, Deserialize)]
pub struct LogEntry {
    pub timestamp: String,
    pub veilkey: String,
    pub pattern: String,
    pub confidence: i32,
}

pub struct SessionLogger {
    path: String,
}

impl SessionLogger {
    pub fn new(path: &str) -> Self {
        Self {
            path: path.to_string(),
        }
    }

    pub fn log(&self, veilkey: &str, pattern: &str, confidence: i32) {
        let entry = LogEntry {
            timestamp: chrono::Local::now().format("%Y-%m-%d %H:%M:%S").to_string(),
            veilkey: veilkey.to_string(),
            pattern: pattern.to_string(),
            confidence,
        };

        let result = (|| -> std::io::Result<()> {
            let mut file = open_private_file(Path::new(&self.path), true)?;
            let data = serde_json::to_string(&entry)?;
            writeln!(file, "{}", data)
        })();
        if result.is_err() {
            eprintln!("[veilkey] cannot write private session log");
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
        match fs::read_to_string(&self.path) {
            Ok(data) => data.lines().filter(|l| !l.is_empty()).count(),
            Err(_) => 0,
        }
    }

    pub fn clear(&self) -> std::io::Result<()> {
        fs::remove_file(&self.path)
    }
}
