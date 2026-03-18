use crate::detector::Stats;
use serde::Serialize;
use std::io::Write;

pub struct Finding {
    pub file: String,
    pub line: usize,
    pub pattern: String,
    pub confidence: i32,
    pub r#match: String,
}

pub struct Formatter<W: Write> {
    format: String,
    writer: W,
    // Buffer serialized items for JSON/SARIF so we can join without trailing comma
    buf: Vec<String>,
}

#[derive(Serialize)]
struct JsonFinding<'a> {
    file: &'a str,
    line: usize,
    pattern: &'a str,
    confidence: i32,
    r#match: &'a str,
}

#[derive(Serialize)]
struct SarifResult<'a> {
    rule_id: &'a str,
    message: SarifMessage<'a>,
    locations: Vec<SarifLocation<'a>>,
}

#[derive(Serialize)]
struct SarifMessage<'a> {
    text: &'a str,
}

#[derive(Serialize)]
struct SarifLocation<'a> {
    physical_location: SarifPhysical<'a>,
}

#[derive(Serialize)]
struct SarifPhysical<'a> {
    artifact_location: SarifArtifact<'a>,
    region: SarifRegion,
}

#[derive(Serialize)]
struct SarifArtifact<'a> {
    uri: &'a str,
}

#[derive(Serialize)]
struct SarifRegion {
    start_line: usize,
}

impl<W: Write> Formatter<W> {
    pub fn new(format: &str, writer: W) -> Self {
        Self { format: format.to_string(), writer, buf: Vec::new() }
    }

    pub fn header(&mut self) {
        match self.format.as_str() {
            "json" | "sarif" => {} // written in footer once we have all items
            _ => {
                let _ = writeln!(self.writer, "\x1b[0;36m{:<20} {:<6} {:<25} {:<8} MATCH\x1b[0m",
                    "FILE", "LINE", "PATTERN", "CONF");
                let _ = writeln!(self.writer, "{}", "─".repeat(80));
            }
        }
    }

    pub fn format_finding(&mut self, f: Finding) {
        match self.format.as_str() {
            "json" => {
                let j = JsonFinding {
                    file: &f.file, line: f.line,
                    pattern: &f.pattern, confidence: f.confidence, r#match: &f.r#match,
                };
                if let Ok(s) = serde_json::to_string(&j) {
                    self.buf.push(format!("  {}", s));
                }
            }
            "sarif" => {
                let r = SarifResult {
                    rule_id: &f.pattern,
                    message: SarifMessage { text: &f.pattern },
                    locations: vec![SarifLocation {
                        physical_location: SarifPhysical {
                            artifact_location: SarifArtifact { uri: &f.file },
                            region: SarifRegion { start_line: f.line },
                        },
                    }],
                };
                if let Ok(s) = serde_json::to_string(&r) {
                    self.buf.push(s);
                }
            }
            _ => {
                let _ = writeln!(self.writer, "\x1b[0;33m{:<20}\x1b[0m {:<6} {:<25} {:<8} {}",
                    f.file, f.line, f.pattern, f.confidence, f.r#match);
            }
        }
    }

    pub fn format_summary(&mut self, stats: &Stats) {
        match self.format.as_str() {
            "json" | "sarif" => {}
            _ => {
                let _ = writeln!(self.writer);
                let _ = writeln!(self.writer, "Lines scanned: {}", stats.lines);
                let _ = writeln!(self.writer, "Secrets found: {}", stats.detections);
            }
        }
    }

    pub fn footer(&mut self) {
        match self.format.as_str() {
            "json" => {
                let _ = writeln!(self.writer, "[");
                let _ = writeln!(self.writer, "{}", self.buf.join(",\n"));
                let _ = writeln!(self.writer, "]");
            }
            "sarif" => {
                let _ = write!(self.writer,
                    r#"{{"version":"2.1.0","runs":[{{"results":[{}]}}]}}"#,
                    self.buf.join(",")
                );
                let _ = writeln!(self.writer);
            }
            _ => {}
        }
    }
}
