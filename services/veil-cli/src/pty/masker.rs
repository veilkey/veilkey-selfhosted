use crate::api::VeilKeyClient;
use crate::config::CompiledPattern;

const BOLD: &str = "\x1b[1m";
const CYAN: &str = "\x1b[36m";
const RED: &str = "\x1b[31m";
const DIM: &str = "\x1b[2m";
const GREEN: &str = "\x1b[92m";
const RESET: &str = "\x1b[0m";

pub fn colorize_ref(vk_ref: &str) -> String {
    if vk_ref.contains(":LOCAL:") {
        format!("{}{}{}{}", BOLD, CYAN, vk_ref, RESET)
    } else if vk_ref.contains(":TEMP:") {
        format!("{}{}{}{}", BOLD, RED, vk_ref, RESET)
    } else {
        vk_ref.to_string()
    }
}

/// VE ref: colorize original value without changing it — just add color.
/// e.g. "soulflow-lv" → (green)"soulflow-lv"(reset) — same text, different color.
pub fn colorize_ve_ref(original: &str, _ve_ref: &str) -> String {
    format!("{}{}{}", GREEN, original, RESET)
}

pub fn padded_colorize_ref(vk_ref: &str, original_len: usize) -> String {
    let colored = colorize_ref(vk_ref);
    let visible_len = vk_ref.len();
    if visible_len < original_len {
        format!("{}{}", colored, " ".repeat(original_len - visible_len))
    } else {
        colored
    }
}

/// Mask secrets in PTY output data.
/// - Known secrets from mask_map are replaced with colorized VK refs.
/// - Lines with replacements get ANSI line-clear to overwrite echo-back.
/// - Pattern-detected secrets are auto-registered or redacted (fail-closed).
pub fn mask_output(
    data: &[u8],
    mask_map: &[(String, String)],
    ve_map: &[(String, String)],
    patterns: &[CompiledPattern],
    client: &VeilKeyClient,
    recent_input: &str,
) -> Vec<u8> {
    let mut s = String::from_utf8_lossy(data).to_string();
    let mut had_replacement = false;

    // 1. Known secrets — replace with colorized VK refs
    for (plaintext, vk_ref) in mask_map {
        if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
            s = s.replace(
                plaintext.as_str(),
                &padded_colorize_ref(vk_ref, plaintext.len()),
            );
            had_replacement = true;
        }
    }

    // Line-clear on lines that had secrets replaced (contain our BOLD+CYAN or BOLD+RED).
    // Only match our colorize_ref output, not pre-existing ANSI codes (e.g. colored prompts).
    if had_replacement {
        let mut cleared = String::new();
        for (i, line) in s.split('\n').enumerate() {
            if i > 0 {
                cleared.push('\n');
            }
            let has_our_ansi = line.contains("\x1b[1m\x1b[36m") || line.contains("\x1b[1m\x1b[31m");
            if has_our_ansi {
                cleared.push_str("\r\x1b[2K");
            }
            cleared.push_str(line);
        }
        s = cleared;
    }

    // 2. Pattern-detected secrets — auto-register or redact
    let scan_copy = s.clone();
    for pat in patterns {
        for caps in pat.regex.captures_iter(&scan_copy) {
            let m = caps
                .get(pat.group.max(1))
                .or_else(|| caps.get(1))
                .unwrap_or_else(|| caps.get(0).unwrap());
            let secret = m.as_str().trim_end_matches(['\r', '\n']);
            if secret.len() < 8 || secret.starts_with("VK:") {
                continue;
            }
            if mask_map.iter().any(|(p, _)| p == secret) {
                continue;
            }
            if !recent_input.is_empty() && recent_input.contains(secret) {
                continue;
            }
            match client.issue(secret) {
                Ok(ref_canonical) => {
                    s = s.replace(secret, &padded_colorize_ref(&ref_canonical, secret.len()));
                }
                Err(e) => {
                    eprintln!(
                        "[veilkey] issue failed for pattern {}: {} — redacting (fail-closed)",
                        pat.name, e
                    );
                    let redacted = format!("[REDACTED:{}]", pat.name);
                    s = s.replace(secret, &padded_colorize_ref(&redacted, secret.len()));
                }
            }
        }
    }

    // 3. VE (config) entries — display-only: original value + colored tag
    for (plaintext, ve_ref) in ve_map {
        if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
            s = s.replace(plaintext.as_str(), &colorize_ve_ref(plaintext, ve_ref));
        }
    }

    s.into_bytes()
}
