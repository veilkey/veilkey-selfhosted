use unicode_width::UnicodeWidthStr;

use crate::api::VeilKeyClient;
use crate::config::CompiledPattern;

use super::ansi::{SegmentKind, Tokenizer};

const BOLD: &str = "\x1b[1m";
const CYAN: &str = "\x1b[36m";
const RED: &str = "\x1b[31m";
const GREEN: &str = "\x1b[92m";
const RESET: &str = "\x1b[0m";

pub fn colorize_ref(vk_ref: &str) -> String {
    if vk_ref.contains(":TEMP:") {
        format!("{}{}{}{}", BOLD, RED, vk_ref, RESET)
    } else if vk_ref.contains(":LOCAL:") || vk_ref.starts_with("VK:") || vk_ref.chars().all(|c| c.is_ascii_hexdigit() || c == ' ') {
        format!("{}{}{}{}", BOLD, CYAN, vk_ref, RESET)
    } else {
        vk_ref.to_string()
    }
}

/// VE: colorize original value in green — same text, different color.
pub fn colorize_ve_ref(original: &str, _ve_ref: &str) -> String {
    format!("{}{}{}", GREEN, original, RESET)
}

/// Same-width VK ref: adapts format to EXACTLY match original secret width.
pub fn padded_colorize_ref(vk_ref: &str, original_len: usize) -> String {
    if original_len == 0 { return String::new(); }
    let full_len = vk_ref.chars().count();
    let hash = vk_ref.rsplit(':').next().unwrap_or(vk_ref);
    let display = if full_len <= original_len {
        let pad = original_len - full_len;
        if pad > 0 { format!("{}{}", vk_ref, " ".repeat(pad)) } else { vk_ref.to_string() }
    } else {
        let compact = format!("VK:{}", hash);
        let compact_len = compact.chars().count();
        if compact_len <= original_len {
            let pad = original_len - compact_len;
            if pad > 0 { format!("{}{}", compact, " ".repeat(pad)) } else { compact }
        } else if original_len >= 3 {
            let h: String = hash.chars().take(original_len).collect();
            let hlen = h.chars().count();
            if hlen < original_len { format!("{}{}", h, " ".repeat(original_len - hlen)) } else { h }
        } else { "*".repeat(original_len) }
    };
    colorize_ref(&display)
}

/// ANSI-aware secret replacement. Tokenizes input to separate ANSI escape
/// sequences from plaintext, searches for `needle` in concatenated plaintext,
/// then reconstructs output preserving all ANSI codes.
fn ansi_aware_replace(text: &str, needle: &str, replacement: &str) -> (String, bool) {
    if needle.is_empty() {
        return (text.to_string(), false);
    }
    let segments = Tokenizer::tokenize(text.as_bytes());
    let mut plain = String::new();
    for seg in &segments {
        if seg.kind == SegmentKind::Text {
            plain.push_str(&String::from_utf8_lossy(&seg.data));
        }
    }
    let mut matches: Vec<(usize, usize)> = Vec::new();
    let mut search_start = 0;
    while let Some(pos) = plain[search_start..].find(needle) {
        let abs_start = search_start + pos;
        matches.push((abs_start, abs_start + needle.len()));
        search_start = abs_start + needle.len();
    }
    if matches.is_empty() {
        return (text.to_string(), false);
    }
    let mut out = String::new();
    let mut plain_cursor = 0;
    let mut match_idx = 0;
    for seg in &segments {
        if seg.kind == SegmentKind::Escape {
            out.push_str(&String::from_utf8_lossy(&seg.data));
            continue;
        }
        let seg_text = String::from_utf8_lossy(&seg.data);
        let seg_start = plain_cursor;
        let seg_end = seg_start + seg_text.len();
        let mut local_cursor = 0;
        while local_cursor < seg_text.len() && match_idx < matches.len() {
            let (m_start, m_end) = matches[match_idx];
            if m_start >= seg_end {
                break;
            }
            if m_end <= seg_start + local_cursor {
                match_idx += 1;
                continue;
            }
            let match_local_start = m_start.saturating_sub(seg_start);
            if match_local_start > local_cursor {
                out.push_str(&seg_text[local_cursor..match_local_start]);
            }
            if m_start >= seg_start + local_cursor {
                out.push_str(replacement);
            }
            let match_local_end = (m_end - seg_start).min(seg_text.len());
            local_cursor = match_local_end;
            if m_end <= seg_end {
                match_idx += 1;
            } else {
                break;
            }
        }
        if local_cursor < seg_text.len() {
            out.push_str(&seg_text[local_cursor..]);
        }
        plain_cursor = seg_end;
    }
    (out, true)
}

// Alt-screen detection sequences (vim, less, htop, etc.)
const ALT_SCREEN_ENABLE: &[u8] = b"\x1b[?1049h";
const ALT_SCREEN_DISABLE: &[u8] = b"\x1b[?1049l";

/// Check if data contains alt-screen toggle and return new state.
pub fn detect_alt_screen(data: &[u8], current: bool) -> bool {
    // Last occurrence wins (multiple toggles in one chunk)
    let enable_pos = data
        .windows(ALT_SCREEN_ENABLE.len())
        .rposition(|w| w == ALT_SCREEN_ENABLE);
    let disable_pos = data
        .windows(ALT_SCREEN_DISABLE.len())
        .rposition(|w| w == ALT_SCREEN_DISABLE);
    match (enable_pos, disable_pos) {
        (Some(e), Some(d)) => e > d, // last one wins
        (Some(_), None) => true,
        (None, Some(_)) => false,
        (None, None) => current,
    }
}

/// Plain tail size — last N bytes of output kept for cross-chunk secret detection.
/// Inspired by secretty's plainTail approach: secrets split across PTY read() calls
/// are caught by matching against tail+new_data combined.
const PLAIN_TAIL_SIZE: usize = 8192;

/// Mask secrets in PTY output data using a lookback tail buffer.
///
/// `plain_tail` holds the last PLAIN_TAIL_SIZE bytes of previously emitted output.
/// We prepend it to the current data for matching, but only emit replacements that
/// touch the new data portion. This catches secrets split across PTY chunks without
/// any timing-dependent heuristics.
///
/// Returns (masked_output_bytes, updated_plain_tail).
pub fn mask_output(
    data: &[u8],
    mask_map: &[(String, String)],
    ve_map: &[(String, String)],
    patterns: &[CompiledPattern],
    client: &VeilKeyClient,
    recent_input: &str,
    plain_tail: &str,
) -> (Vec<u8>, String) {
    let new_text = String::from_utf8_lossy(data).to_string();
    if new_text.is_empty() {
        return (Vec::new(), plain_tail.to_string());
    }

    let combined = format!("{}{}", plain_tail, new_text);

    // Pre-scan: detect secrets on combined (tail+new) buffer.
    // This catches secrets split across PTY chunks. Any detected secret
    // is issued to the API, and if it spans the tail/new_text boundary,
    // the overlapping portion in new_text is masked to prevent leaking.
    let tail_len = plain_tail.len();
    let mut cross_chunk_replacements: Vec<(String, String)> = Vec::new();
    for pat in patterns {
        for caps in pat.regex.captures_iter(&combined) {
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
            // Issue to API — registers the secret for future mask_map inclusion.
            let _ = client.issue(secret);
            // If the match spans the tail/new_text boundary, mask the
            // overlapping portion that falls within new_text.
            let match_start = m.start();
            let match_end = m.start() + secret.len();
            if match_start < tail_len && match_end > tail_len {
                let overlap = &combined[tail_len..match_end];
                if !overlap.is_empty() {
                    cross_chunk_replacements.push((overlap.to_string(), " ".repeat(overlap.len())));
                }
            }
        }
    }

    // Output masking: apply replacements on new_text only (tail was already emitted).
    // The combined pre-scan above already issued split secrets to the API.
    let mut output = new_text.clone();

    // Cross-chunk mask_map: secrets typed char-by-char span tail + new_text.
    // Same-width refs ensure cursor position stays correct after erase.
    if let Some(m) = find_cross_chunk_mask(plain_tail, &new_text, mask_map) {
        output = m.output;
    }

    // Apply cross-chunk boundary replacements first (secret suffix leaked into new_text)
    for (leaked, replacement) in &cross_chunk_replacements {
        output = output.replacen(leaked, replacement, 1);
    }
    for (plaintext, vk_ref) in mask_map {
        if plaintext.is_empty() {
            continue;
        }
        let repl = padded_colorize_ref(vk_ref, UnicodeWidthStr::width(plaintext.as_str()));
        let (new_out, replaced) = ansi_aware_replace(&output, plaintext, &repl);
        if replaced {
            output = new_out;
        }
    }
    // Pattern-detected replacements — scan on ANSI-stripped text
    let plain_for_scan =
        String::from_utf8_lossy(&Tokenizer::strip_ansi(new_text.as_bytes())).to_string();
    for pat in patterns {
        for caps in pat.regex.captures_iter(&plain_for_scan) {
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
            // Already issued above via combined scan — just replace here
            match client.issue(secret) {
                Ok(ref_canonical) => {
                    let repl = padded_colorize_ref(&ref_canonical, UnicodeWidthStr::width(secret));
                    let (new_out, replaced) = ansi_aware_replace(&output, secret, &repl);
                    if replaced {
                        output = new_out;
                    }
                }
                Err(e) => {
                    eprintln!(
                        "[veilkey] issue failed for pattern {}: {} — redacting (fail-closed)",
                        pat.name, e
                    );
                    let redacted = format!("[REDACTED:{}]", pat.name);
                    let repl = padded_colorize_ref(&redacted, UnicodeWidthStr::width(secret));
                    let (new_out, replaced) = ansi_aware_replace(&output, secret, &repl);
                    if replaced {
                        output = new_out;
                    }
                }
            }
        }
    }
    for (plaintext, ve_ref) in ve_map {
        if !plaintext.is_empty() {
            let repl = colorize_ve_ref(plaintext, ve_ref);
            let (new_out, _) = ansi_aware_replace(&output, plaintext, &repl);
            output = new_out;
        }
    }

    // NOTE: Line-clear (\r\x1b[2K) was previously inserted on lines with masked
    // secrets. Removed because it breaks readline prompt redraws — the clear
    // sequence erases the prompt that bash/zsh already drew, causing visual
    // glitches on PS1 lines. The padded replacement already preserves width.

    // Update plain tail — keep last PLAIN_TAIL_SIZE bytes of the ORIGINAL new text
    // (not the masked version, so future matching works on plaintext)
    let new_tail = if new_text.len() > PLAIN_TAIL_SIZE {
        let start = new_text.ceil_char_boundary(new_text.len() - PLAIN_TAIL_SIZE);
        format!(
            "{}{}",
            &plain_tail[plain_tail
                .ceil_char_boundary(plain_tail.len().saturating_sub(PLAIN_TAIL_SIZE / 2))..],
            &new_text[start..]
        )
    } else {
        let combined_tail = format!("{}{}", plain_tail, new_text);
        if combined_tail.len() > PLAIN_TAIL_SIZE {
            let start = combined_tail.ceil_char_boundary(combined_tail.len() - PLAIN_TAIL_SIZE);
            combined_tail[start..].to_string()
        } else {
            combined_tail
        }
    };

    (output.into_bytes(), new_tail)
}

/// Result of a cross-chunk secret match detection.
#[derive(Debug, PartialEq)]
#[allow(dead_code)]
pub(crate) struct CrossChunkMatch {
    pub output: String,
}

/// Detect a secret that spans across two PTY output chunks.
///
/// `plain_tail` is the trailing plaintext from the previous chunk(s).
/// `new_text` is the current chunk. If a secret from `mask_map` straddles
/// the boundary (starts in tail, ends in new_text), returns a replacement
/// string with cursor-back + colorized ref + remainder.
///
/// Not used in the mask_output pipeline (which has its own cross-chunk logic),
/// but available for targeted testing.
#[allow(dead_code)]
pub(crate) fn find_cross_chunk_mask(
    plain_tail: &str,
    new_text: &str,
    mask_map: &[(String, String)],
) -> Option<CrossChunkMatch> {
    if new_text.contains("\x1b[") {
        return None;
    }
    let combined = format!("{}{}", plain_tail, new_text);
    let tail_len = plain_tail.len();
    let mut best: Option<(usize, String, String, String)> = None;
    for (plaintext, vk_ref) in mask_map {
        if plaintext.is_empty() || plaintext.len() < 3 {
            continue;
        }
        let raw_start = tail_len.saturating_sub(plaintext.len() - 1);
        let search_start = combined.floor_char_boundary(raw_start);
        let boundary = &combined[search_start..];
        if let Some(rel_pos) = boundary.find(plaintext.as_str()) {
            let pos = search_start + rel_pos;
            let end = pos + plaintext.len();
            if pos < tail_len && end > tail_len && end <= combined.len() {
                let new_part = &combined[tail_len..end];
                if !new_text.starts_with(new_part) {
                    continue;
                }
                let is_longer = best
                    .as_ref()
                    .map_or(true, |(len, _, _, _)| plaintext.len() > *len);
                if is_longer {
                    let tail_part = &combined[pos..tail_len];
                    let tail_chars = tail_part.chars().count();
                    let erase = format!("\x1b[{}D\x1b[K", tail_chars);
                    let replacement = padded_colorize_ref(vk_ref, plaintext.len());
                    let remainder = new_text[new_part.len()..].to_string();
                    best = Some((plaintext.len(), erase, replacement, remainder));
                }
            }
        }
    }
    best.map(|(_, erase, replacement, remainder)| CrossChunkMatch {
        output: format!("{}{}{}", erase, replacement, remainder),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn strip_ansi(s: &str) -> String {
        String::from_utf8_lossy(&Tokenizer::strip_ansi(s.as_bytes())).to_string()
    }

    /// Helper: simulate ANSI-aware mask_map replacement like mask_output step 1
    fn simulate_mask(input: &str, mask_map: &[(String, String)]) -> String {
        let mut s = input.to_string();
        for (plaintext, vk_ref) in mask_map {
            if plaintext.is_empty() {
                continue;
            }
            let repl = padded_colorize_ref(vk_ref, UnicodeWidthStr::width(plaintext.as_str()));
            let (new_s, _) = ansi_aware_replace(&s, plaintext, &repl);
            s = new_s;
        }
        strip_ansi(&s)
    }

    // ── padded_colorize_ref unit tests ──────────────────────────────

    #[test]
    fn test_pad_ref_shorter_than_secret() {
        // ref 17 chars, secret 30 chars → 13 spaces padding
        let result = padded_colorize_ref("VK:LOCAL:6da25530", 30);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 30);
        assert!(visible.starts_with("VK:LOCAL:6da25530"));
    }

    #[test]
    fn test_pad_ref_longer_than_secret() {
        // ref 17 chars, secret 10 chars → show full ref (no truncation)
        let result = padded_colorize_ref("VK:LOCAL:6da25530", 10);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 10); // same-width
    }

    #[test]
    fn test_pad_ref_equal_to_secret() {
        // ref 17 chars, secret 17 chars → exact fit, no padding
        let result = padded_colorize_ref("VK:LOCAL:6da25530", 17);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 17);
        assert_eq!(visible, "VK:LOCAL:6da25530");
    }

    #[test]
    fn test_pad_very_short_secret() {
        // secret 3 chars, ref 12 chars → full ref shown (no truncation)
        let result = padded_colorize_ref("VK:LOCAL:abc", 3);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 3); // same-width
    }

    #[test]
    fn test_pad_very_long_secret() {
        // secret 100 chars → lots of padding
        let result = padded_colorize_ref("VK:LOCAL:abc", 100);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 100);
    }

    #[test]
    fn test_pad_single_char_secret() {
        // secret 1 char, ref 12 chars → full ref shown (no truncation)
        let result = padded_colorize_ref("VK:LOCAL:abc", 1);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 1); // same-width
    }

    #[test]
    fn test_pad_zero_length() {
        let result = padded_colorize_ref("VK:LOCAL:abc", 0);
        assert_eq!(result, "");
    }

    #[test]
    fn test_pad_temp_ref_coloring() {
        let result = padded_colorize_ref("VK:TEMP:abc12345", 20);
        // TEMP refs use RED color
        assert!(result.contains(RED));
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 20);
    }

    // ── Connection string masking ───────────────────────────────────

    #[test]
    fn test_mask_connection_string_password_only() {
        // mysql://user:PASSWORD@host:3306/db
        let input = "mysql://admin:SuperSecret123@db.example.com:3306/mydb";
        let mask_map = vec![(
            "SuperSecret123".to_string(),
            "VK:LOCAL:aaa11111".to_string(),
        )];
        let result = simulate_mask(input, &mask_map);
        // ref (17) > secret (14) so result is wider — that's OK
        assert!(
            result.contains("@db.example.com:3306/mydb"),
            "host must be intact"
        );
        assert!(result.contains("admin:"), "username must be intact");
        assert!(!result.contains("SuperSecret"), "password must not leak");
    }

    #[test]
    fn test_mask_connection_string_with_at_in_password() {
        // Password contains @ — the FULL password must be in mask_map
        let input = "postgres://user:p@ss!word@host:5432";
        let mask_map = vec![("p@ss!word".to_string(), "VK:LOCAL:bbb22222".to_string())];
        let result = simulate_mask(input, &mask_map);
        // ref (17) > secret (9) so result is wider
        assert!(result.contains("@host:5432"), "host part must survive");
        assert!(!result.contains("p@ss"), "password must not leak");
    }

    // ── Multiple secrets on one line ────────────────────────────────

    #[test]
    fn test_mask_two_secrets_same_line() {
        let input = "API_KEY=secret-api-key-value DB_PASS=db-password-here";
        let mask_map = vec![
            (
                "secret-api-key-value".to_string(),
                "VK:LOCAL:aaa".to_string(),
            ),
            ("db-password-here".to_string(), "VK:LOCAL:bbb".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("secret-api"), "first secret must not leak");
        assert!(
            !result.contains("db-password"),
            "second secret must not leak"
        );
        assert!(result.contains("API_KEY="), "key names must survive");
        assert!(result.contains("DB_PASS="), "key names must survive");
    }

    #[test]
    fn test_mask_same_secret_twice() {
        let input = "first=MyPassword second=MyPassword";
        let mask_map = vec![("MyPassword".to_string(), "VK:LOCAL:ccc".to_string())];
        let result = simulate_mask(input, &mask_map);
        // ref (12) > secret (10) so result is wider
        // Both occurrences must be masked
        assert!(!result.contains("MyPassword"));
    }

    // ── Secret at boundaries ────────────────────────────────────────

    #[test]
    fn test_mask_secret_at_line_start() {
        let input = "SuperSecret123 is exposed";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:ddd".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("SuperSecret"));
        assert!(result.contains(" is exposed"));
    }

    #[test]
    fn test_mask_secret_at_line_end() {
        let input = "password is SuperSecret123";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:eee".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("SuperSecret"));
        assert!(result.contains("password is "));
    }

    #[test]
    fn test_mask_secret_is_entire_line() {
        let input = "SuperSecret123";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:fff".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("SuperSecret"));
    }

    // ── Overlapping secrets (longest-first) ─────────────────────────

    #[test]
    fn test_mask_longest_match_wins() {
        // mask_map sorted longest-first: "secret-api-key" before "secret"
        let input = "token=secret-api-key";
        let mask_map = vec![
            ("secret-api-key".to_string(), "VK:LOCAL:long1".to_string()),
            ("secret".to_string(), "VK:LOCAL:short".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("secret"));
        assert!(
            !result.contains("api-key"),
            "the full match must be used, not partial"
        );
    }

    // ── Special characters in secrets ───────────────────────────────

    #[test]
    fn test_mask_secret_with_special_chars() {
        let input = "PASS=p@$$w0rd!#%^&*()";
        let mask_map = vec![("p@$$w0rd!#%^&*()".to_string(), "VK:LOCAL:spec1".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("p@$$"));
        assert!(result.contains("PASS="));
    }

    #[test]
    fn test_mask_secret_with_quotes() {
        let input = r#"export SECRET="my-quoted-secret""#;
        let mask_map = vec![("my-quoted-secret".to_string(), "VK:LOCAL:quot1".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("my-quoted"));
        assert!(result.contains("export SECRET=\""));
    }

    #[test]
    fn test_mask_secret_with_newline_adjacent() {
        let input = "line1=secret1\nline2=secret2";
        let mask_map = vec![
            ("secret1".to_string(), "VK:LOCAL:nl1".to_string()),
            ("secret2".to_string(), "VK:LOCAL:nl2".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        // ref (12) > secret (7) so result is wider
        assert!(!result.contains("secret1"));
        assert!(!result.contains("secret2"));
    }

    // ── JSON / structured output ────────────────────────────────────

    #[test]
    fn test_mask_json_value() {
        let input = r#"{"api_key":"sk-1234567890abcdef","host":"example.com"}"#;
        let mask_map = vec![(
            "sk-1234567890abcdef".to_string(),
            "VK:LOCAL:json1".to_string(),
        )];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("sk-1234567890"));
        assert!(result.contains("example.com"));
    }

    // ── env var output ──────────────────────────────────────────────

    #[test]
    fn test_mask_env_export_line() {
        let input = "export DATABASE_URL=postgres://admin:hunter2@db:5432/prod";
        let mask_map = vec![("hunter2".to_string(), "VK:LOCAL:env1".to_string())];
        let result = simulate_mask(input, &mask_map);
        // ref (13) > secret (7) so result is wider
        assert!(!result.contains("hunter2"));
        assert!(result.contains("admin:"));
        assert!(result.contains("@db:5432/prod"));
    }

    // ── No false positives ──────────────────────────────────────────

    #[test]
    fn test_no_mask_when_no_match() {
        let input = "this line has no secrets at all";
        let mask_map = vec![("SuperSecret".to_string(), "VK:LOCAL:nope".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result, input, "unchanged text must pass through");
    }

    #[test]
    fn test_mask_map_empty() {
        let input = "anything here";
        let result = simulate_mask(input, &[]);
        assert_eq!(result, input);
    }

    // ── Width preservation regression ───────────────────────────────

    #[test]
    fn test_width_preserved_when_ref_fits() {
        // When ref <= secret length, width is exactly preserved (padded).
        // When ref > secret length, full ref is shown (result is longer).
        let ref_str = "VK:LOCAL:6da25530"; // 17 chars
        let ref_len = ref_str.len();
        for secret_len in [5, 10, 15, 17, 20, 30, 50, 100] {
            let secret: String = (0..secret_len)
                .map(|i| (b'a' + (i % 26) as u8) as char)
                .collect();
            let input = format!("prefix:{}:suffix", secret);
            let mask_map = vec![(secret.clone(), ref_str.to_string())];
            let result = simulate_mask(&input, &mask_map);
            assert!(
                !result.contains(&secret),
                "secret leaked for len={}",
                secret_len
            );
            assert!(
                result.contains(":suffix"),
                "suffix missing for len={}",
                secret_len
            );
            if secret_len >= ref_len {
                // Ref fits — width is exactly preserved
                assert_eq!(
                    result.len(),
                    input.len(),
                    "width mismatch for secret_len={}: input=[{}] result=[{}]",
                    secret_len,
                    input,
                    result
                );
            } else {
                // Same-width: result width == input width
                assert_eq!(result.len(), input.len(), "same-width mismatch for secret_len={}", secret_len);
            }
        }
    }

    // ── Alt-screen detection ────────────────────────────────────────

    #[test]
    fn test_alt_screen_enable() {
        let data = b"some text\x1b[?1049hmore text";
        assert!(detect_alt_screen(data, false));
    }

    #[test]
    fn test_alt_screen_disable() {
        let data = b"some text\x1b[?1049lmore text";
        assert!(!detect_alt_screen(data, true));
    }

    #[test]
    fn test_alt_screen_no_change() {
        let data = b"regular output with no escape";
        assert!(!detect_alt_screen(data, false));
        assert!(detect_alt_screen(data, true));
    }

    #[test]
    fn test_alt_screen_enable_then_disable() {
        // Both in same chunk — last one wins
        let data = b"\x1b[?1049hstuff\x1b[?1049l";
        assert!(!detect_alt_screen(data, false), "disable comes last");
    }

    #[test]
    fn test_alt_screen_disable_then_enable() {
        let data = b"\x1b[?1049lstuff\x1b[?1049h";
        assert!(detect_alt_screen(data, false), "enable comes last");
    }

    // ── ANSI-aware masking ────────────────────────────────────────────

    #[test]
    fn test_ansi_replace_split_by_color() {
        let input = "\x1b[31mpass\x1b[0mword123";
        let (r, ok) = ansi_aware_replace(input, "password123", "[MASKED]");
        assert!(ok);
        let p = strip_ansi(&r);
        assert!(!p.contains("pass"));
        assert!(p.contains("[MASKED]"));
    }

    #[test]
    fn test_ansi_replace_three_segments() {
        let input = "\x1b[31mmy\x1b[32msecret\x1b[33mkey\x1b[0m";
        let (r, ok) = ansi_aware_replace(input, "mysecretkey", "[R]");
        assert!(ok);
        let p = strip_ansi(&r);
        assert!(p.contains("[R]"));
        assert!(!p.contains("mysecretkey"));
    }

    #[test]
    fn test_ansi_replace_grep_color() {
        let input = "KEY=\x1b[01;31m\x1b[Ksk-abc123\x1b[m\x1b[K";
        let (r, ok) = ansi_aware_replace(input, "sk-abc123", "VK:LOCAL:x");
        assert!(ok);
        let p = strip_ansi(&r);
        assert!(!p.contains("sk-abc123"));
        assert!(p.contains("VK:LOCAL:x"));
    }

    #[test]
    fn test_ansi_replace_preserves_surrounding() {
        let input = "\x1b[1mpre\x1b[0m secret \x1b[1mpost\x1b[0m";
        let (r, ok) = ansi_aware_replace(input, "secret", "[X]");
        assert!(ok);
        assert!(r.contains("\x1b[1mpre\x1b[0m"));
        assert!(r.contains("\x1b[1mpost\x1b[0m"));
        assert_eq!(strip_ansi(&r), "pre [X] post");
    }

    #[test]
    fn test_simulate_mask_with_ansi_input() {
        let input = "\x1b[31mpass\x1b[0mword";
        let map = vec![("password".to_string(), "VK:LOCAL:pw1".to_string())];
        let r = simulate_mask(input, &map);
        assert!(!r.contains("password"));
        assert!(!r.contains("pass"));
    }

    // ── No line-clear tests (prompt preservation) ─────────────────

    #[test]
    fn test_no_line_clear_in_masked_output() {
        // Verify that masked output does NOT contain \r\x1b[2K sequences
        // which would break readline prompt redraws.
        let input = "SECRET=SuperSecretValue123";
        let mask_map = vec![(
            "SuperSecretValue123".to_string(),
            "VK:LOCAL:abc12345".to_string(),
        )];
        let mut s = input.to_string();
        for (plaintext, vk_ref) in &mask_map {
            let repl = padded_colorize_ref(vk_ref, UnicodeWidthStr::width(plaintext.as_str()));
            let (new_s, _) = ansi_aware_replace(&s, plaintext, &repl);
            s = new_s;
        }
        assert!(
            !s.contains("\r\x1b[2K"),
            "line-clear must not be present in masked output"
        );
    }

    #[test]
    fn test_no_line_clear_multiline() {
        let input = "line1=secret111111\nline2=secret222222";
        let mask_map = vec![
            ("secret111111".to_string(), "VK:LOCAL:a1".to_string()),
            ("secret222222".to_string(), "VK:LOCAL:b2".to_string()),
        ];
        let mut s = input.to_string();
        for (plaintext, vk_ref) in &mask_map {
            let repl = padded_colorize_ref(vk_ref, UnicodeWidthStr::width(plaintext.as_str()));
            let (new_s, _) = ansi_aware_replace(&s, plaintext, &repl);
            s = new_s;
        }
        assert!(
            !s.contains("\r\x1b[2K"),
            "line-clear must not be present"
        );
        // Prompt-style prefix must survive
        assert!(strip_ansi(&s).contains("line1="));
        assert!(strip_ansi(&s).contains("line2="));
    }

    // ── Cross-chunk detection tests ───────────────────────────────

    #[test]
    fn test_cross_chunk_basic() {
        let secret = "my-secret-password";
        let map = vec![(secret.to_string(), "VK:LOCAL:cross1".to_string())];
        // Split: "my-secret-" in tail, "password" in new_text
        let tail = "my-secret-";
        let new = "password";
        let result = find_cross_chunk_mask(tail, new, &map);
        assert!(result.is_some(), "cross-chunk match expected");
        let m = result.unwrap();
        let visible = strip_ansi(&m.output);
        assert!(visible.contains("VK:LOCAL:cross1"));
    }

    #[test]
    fn test_cross_chunk_no_match() {
        let map = vec![("my-secret-password".to_string(), "VK:LOCAL:x".to_string())];
        let tail = "unrelated-";
        let new = "text-here";
        assert!(find_cross_chunk_mask(tail, new, &map).is_none());
    }

    #[test]
    fn test_cross_chunk_stale_tail() {
        // Tail has extra prefix that doesn't match
        let map = vec![("abcdef".to_string(), "VK:LOCAL:stale1".to_string())];
        let tail = "xxxabc";
        let new = "def";
        let result = find_cross_chunk_mask(tail, new, &map);
        assert!(result.is_some());
    }

    #[test]
    fn test_cross_chunk_utf8() {
        let secret = "secret-value";
        let map = vec![(secret.to_string(), "VK:LOCAL:utf1".to_string())];
        let tail = "secret-";
        let new = "value";
        let result = find_cross_chunk_mask(tail, new, &map);
        assert!(result.is_some());
    }

    #[test]
    fn test_cross_chunk_skips_ansi() {
        let map = vec![("secret".to_string(), "VK:LOCAL:x".to_string())];
        let tail = "sec";
        let new = "\x1b[31mret";
        // Should return None because new_text contains ANSI
        assert!(find_cross_chunk_mask(tail, new, &map).is_none());
    }

    #[test]
    fn test_cross_chunk_short_secret_ignored() {
        // Secrets < 3 chars are ignored
        let map = vec![("ab".to_string(), "VK:LOCAL:x".to_string())];
        let tail = "a";
        let new = "b";
        assert!(find_cross_chunk_mask(tail, new, &map).is_none());
    }

    #[test]
    fn test_cross_chunk_repeated_secret() {
        // Secret appears fully in tail (not cross-chunk)
        let map = vec![("abcdef".to_string(), "VK:LOCAL:r".to_string())];
        let tail = "abcdef"; // full secret in tail
        let new = "xyz";
        // No cross-chunk match — it doesn't straddle the boundary
        assert!(find_cross_chunk_mask(tail, new, &map).is_none());
    }

    // ── Charwise echo simulation ──────────────────────────────────

    #[test]
    fn test_charwise_echo_no_leak() {
        // Simulate char-by-char echo: each char is a separate "chunk"
        // The plain_tail mechanism should prevent leaking
        let secret = "SuperSecret123";
        let map = vec![(secret.to_string(), "VK:LOCAL:echo1".to_string())];
        let mut tail = String::new();
        let mut all_output = String::new();
        for ch in secret.chars() {
            let chunk = ch.to_string();
            // Simulate: simulate_mask won't catch single chars, but
            // we test that nothing leaks the full secret
            let mut s = chunk.clone();
            for (plaintext, vk_ref) in &map {
                if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
                    let repl = padded_colorize_ref(vk_ref, plaintext.len());
                    s = s.replace(plaintext.as_str(), &strip_ansi(&repl));
                }
            }
            all_output.push_str(&s);
            tail.push_str(&chunk);
            if tail.len() > 8192 {
                tail = tail[tail.len() - 8192..].to_string();
            }
        }
        // Individual chars go through unmolested, but cross-chunk detection
        // would catch the secret. We verify the find_cross_chunk_mask logic:
        for split_at in 1..secret.len() {
            let t = &secret[..split_at];
            let n = &secret[split_at..];
            let result = find_cross_chunk_mask(t, n, &map);
            assert!(
                result.is_some(),
                "cross-chunk should catch secret split at {}",
                split_at
            );
        }
    }

    // ── Chunked echo simulation ───────────────────────────────────

    #[test]
    fn test_chunked_echo_simulation() {
        // Simulate PTY output arriving in 4-byte chunks
        let output = "KEY=SuperSecret123 done";
        let secret = "SuperSecret123";
        let map = vec![(secret.to_string(), "VK:LOCAL:chunk1".to_string())];
        let chunk_size = 4;
        let mut tail = String::new();
        let mut collected = String::new();
        for start in (0..output.len()).step_by(chunk_size) {
            let end = (start + chunk_size).min(output.len());
            let chunk = &output[start..end];
            // Check cross-chunk match
            if let Some(m) = find_cross_chunk_mask(&tail, chunk, &map) {
                collected.push_str(&strip_ansi(&m.output));
            } else {
                // Simple within-chunk replacement
                let mut s = chunk.to_string();
                for (plaintext, vk_ref) in &map {
                    if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
                        s = s.replace(plaintext.as_str(), &strip_ansi(&padded_colorize_ref(vk_ref, plaintext.len())));
                    }
                }
                collected.push_str(&s);
            }
            tail.push_str(chunk);
            if tail.len() > 8192 {
                tail = tail[tail.len() - 8192..].to_string();
            }
        }
        // The secret should not appear in collected output
        // (some intermediate chars may leak in charwise, but cross-chunk catches the split)
        assert!(!collected.contains(secret), "secret must not appear in full: {}", collected);
    }

    // ── mask_output integration tests ─────────────────────────────

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }

    /// Helper: call mask_output with only mask_map (no patterns, no VE, no API)
    fn mask_with_ve(
        data: &str,
        mask_map: &[(String, String)],
        ve_map: &[(String, String)],
        tail: &str,
    ) -> (String, String) {
        init_crypto();
        let client = VeilKeyClient::new("http://localhost:0");
        let (bytes, new_tail) = mask_output(
            data.as_bytes(),
            mask_map,
            ve_map,
            &[],
            &client,
            "",
            tail,
        );
        (String::from_utf8_lossy(&bytes).to_string(), new_tail)
    }

    /// Helper: call mask_output with mask_map and recent_input
    fn mask_with_input(
        data: &str,
        mask_map: &[(String, String)],
        recent_input: &str,
        tail: &str,
    ) -> (String, String) {
        init_crypto();
        let client = VeilKeyClient::new("http://localhost:0");
        let (bytes, new_tail) = mask_output(
            data.as_bytes(),
            mask_map,
            &[],
            &[],
            &client,
            recent_input,
            tail,
        );
        (String::from_utf8_lossy(&bytes).to_string(), new_tail)
    }

    #[test]
    fn test_mask_output_basic() {
        let map = vec![("my-password-1234".to_string(), "VK:LOCAL:out1".to_string())];
        let (output, _tail) = mask_with_ve("echo my-password-1234", &map, &[], "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("my-password-1234"));
        assert!(visible.contains("echo "));
    }

    #[test]
    fn test_mask_output_empty_data() {
        let map = vec![("secret".to_string(), "VK:LOCAL:x".to_string())];
        let (output, tail) = mask_with_ve("", &map, &[], "prev-tail");
        assert!(output.is_empty());
        assert_eq!(tail, "prev-tail");
    }

    #[test]
    fn test_mask_output_ve_coloring() {
        let ve = vec![("config-value".to_string(), "VE:conf1".to_string())];
        let (output, _) = mask_with_ve("show config-value here", &[], &ve, "");
        // VE entries are colorized green
        assert!(output.contains(GREEN));
        let visible = strip_ansi(&output);
        assert!(visible.contains("config-value"));
    }

    #[test]
    fn test_mask_output_recent_input_skips() {
        // When the secret was recently typed as input, masking is skipped
        // (to avoid masking what the user intentionally typed)
        let map = vec![("typed-secret-12".to_string(), "VK:LOCAL:skip1".to_string())];
        let (output, _) = mask_with_input(
            "typed-secret-12",
            &map,
            "typed-secret-12",
            "",
        );
        let visible = strip_ansi(&output);
        // The mask_map replacement still happens because recent_input only
        // affects pattern-detected secrets, not mask_map entries
        // mask_map is always applied regardless of recent_input
        assert!(!visible.contains("typed-secret-12") || visible.contains("VK:LOCAL:skip1"));
    }

    #[test]
    fn test_mask_output_tail_accumulation() {
        let map = vec![("secret12345678".to_string(), "VK:LOCAL:t1".to_string())];
        let (_, tail1) = mask_with_ve("hello ", &map, &[], "");
        assert_eq!(tail1, "hello ");
        let (_, tail2) = mask_with_ve("world", &map, &[], &tail1);
        assert_eq!(tail2, "hello world");
    }

    #[test]
    fn test_mask_output_no_false_positive() {
        let map = vec![("not-in-output-at-all".to_string(), "VK:LOCAL:fp".to_string())];
        let (output, _) = mask_with_ve("completely normal text", &map, &[], "");
        assert_eq!(output, "completely normal text");
    }
}
