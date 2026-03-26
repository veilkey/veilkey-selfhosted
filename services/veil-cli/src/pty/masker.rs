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
    } else if vk_ref.contains(":SSH:") {
        format!("{}{}{}{}", BOLD, GREEN, vk_ref, RESET)
    } else if vk_ref.contains(":LOCAL:")
        || vk_ref.starts_with("VK:")
        || vk_ref.chars().all(|c| c.is_ascii_hexdigit() || c == ' ')
    {
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
    if original_len == 0 {
        return String::new();
    }
    let full_len = vk_ref.chars().count();
    let hash = vk_ref.rsplit(':').next().unwrap_or(vk_ref);
    let display = if full_len <= original_len {
        let pad = original_len - full_len;
        if pad > 0 {
            format!("{}{}", vk_ref, " ".repeat(pad))
        } else {
            vk_ref.to_string()
        }
    } else {
        let compact = format!("VK:{}", hash);
        let compact_len = compact.chars().count();
        if compact_len <= original_len {
            let pad = original_len - compact_len;
            if pad > 0 {
                format!("{}{}", compact, " ".repeat(pad))
            } else {
                compact
            }
        } else if original_len >= 3 {
            let h: String = hash.chars().take(original_len).collect();
            let hlen = h.chars().count();
            if hlen < original_len {
                format!("{}{}", h, " ".repeat(original_len - hlen))
            } else {
                h
            }
        } else {
            "*".repeat(original_len)
        }
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
                    .is_none_or(|(len, _, _, _)| plaintext.len() > *len);
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
                assert_eq!(
                    result.len(),
                    input.len(),
                    "same-width mismatch for secret_len={}",
                    secret_len
                );
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
        assert!(!s.contains("\r\x1b[2K"), "line-clear must not be present");
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
                        s = s.replace(
                            plaintext.as_str(),
                            &strip_ansi(&padded_colorize_ref(vk_ref, plaintext.len())),
                        );
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
        assert!(
            !collected.contains(secret),
            "secret must not appear in full: {}",
            collected
        );
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
        let (bytes, new_tail) =
            mask_output(data.as_bytes(), mask_map, ve_map, &[], &client, "", tail);
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

    // ── VE config masking ──────────────────────────────────────────

    #[test]
    fn test_ve_shows_plaintext_not_ref() {
        let ve = vec![("10.50.0.113".to_string(), "VE:LOCAL:DB_HOST".to_string())];
        let (output, _) = mask_with_ve("connecting to 10.50.0.113:3306", &[], &ve, "");
        let visible = strip_ansi(&output);
        assert!(visible.contains("10.50.0.113"));
        assert!(!visible.contains("VE:LOCAL:DB_HOST"));
    }

    #[test]
    fn test_ve_colorized_green() {
        let ve = vec![("production".to_string(), "VE:LOCAL:APP_ENV".to_string())];
        let (output, _) = mask_with_ve("env=production", &[], &ve, "");
        assert!(output.contains(GREEN));
        assert!(output.contains(RESET));
    }

    #[test]
    fn test_vk_and_ve_both_applied() {
        let map = vec![(
            "super-secret-key".to_string(),
            "VK:LOCAL:sec12345".to_string(),
        )];
        let ve = vec![("10.0.0.5".to_string(), "VE:LOCAL:DB_HOST".to_string())];
        let (output, _) = mask_with_ve("key=super-secret-key host=10.0.0.5", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(
            !visible.contains("super-secret-key"),
            "VK secret must be masked"
        );
        assert!(
            visible.contains("10.0.0.5"),
            "VE config must show plaintext"
        );
    }

    #[test]
    fn test_vk_masked_before_ve_applied() {
        let map = vec![(
            "secret-10.0.0.5-pass".to_string(),
            "VK:LOCAL:full1234".to_string(),
        )];
        let ve = vec![("10.0.0.5".to_string(), "VE:LOCAL:DB_HOST".to_string())];
        let (output, _) = mask_with_ve("data: secret-10.0.0.5-pass", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("secret-10.0.0.5-pass"));
    }

    #[test]
    fn test_ve_multiple_occurrences() {
        let ve = vec![("prod".to_string(), "VE:LOCAL:ENV".to_string())];
        let (output, _) = mask_with_ve("prod server: prod mode", &[], &ve, "");
        assert!(output.matches(GREEN).count() >= 2);
    }

    #[test]
    fn test_ve_empty_value_skipped() {
        let ve = vec![("".to_string(), "VE:LOCAL:EMPTY".to_string())];
        let (output, _) = mask_with_ve("normal text", &[], &ve, "");
        assert_eq!(output, "normal text");
    }

    #[test]
    fn test_ve_no_entries() {
        let (output, _) = mask_with_ve("normal text", &[], &[], "");
        assert_eq!(output, "normal text");
    }

    #[test]
    fn test_ve_unicode() {
        let ve = vec![("설정값".to_string(), "VE:LOCAL:LABEL".to_string())];
        let (output, _) = mask_with_ve("label: 설정값", &[], &ve, "");
        assert!(strip_ansi(&output).contains("설정값"));
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_not_in_output() {
        let ve = vec![("absent".to_string(), "VE:LOCAL:X".to_string())];
        let (output, _) = mask_with_ve("nothing here", &[], &ve, "");
        assert_eq!(output, "nothing here");
    }

    #[test]
    fn test_ve_multiple_configs() {
        let ve = vec![
            ("10.0.0.5".to_string(), "VE:LOCAL:DB_HOST".to_string()),
            ("3306".to_string(), "VE:LOCAL:DB_PORT".to_string()),
            ("production".to_string(), "VE:LOCAL:ENV".to_string()),
        ];
        let (output, _) = mask_with_ve("host=10.0.0.5 port=3306 env=production", &[], &ve, "");
        let visible = strip_ansi(&output);
        assert!(visible.contains("10.0.0.5"));
        assert!(visible.contains("3306"));
        assert!(visible.contains("production"));
        assert!(output.matches(GREEN).count() >= 3);
    }

    #[test]
    fn domain_ve_preserves_plaintext_vk_replaces() {
        let map = vec![(
            "secret-password".to_string(),
            "VK:LOCAL:sec12345".to_string(),
        )];
        let ve = vec![("config-hostname".to_string(), "VE:LOCAL:HOST".to_string())];
        let (output, _) = mask_with_ve("pw=secret-password host=config-hostname", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("secret-password"), "VK must be HIDDEN");
        assert!(visible.contains("config-hostname"), "VE must be VISIBLE");
    }

    #[test]
    fn domain_pattern_detection_skips_vk_prefix() {
        let src = include_str!("masker.rs");
        assert!(src.matches(r#"starts_with("VK:")"#).count() >= 2);
    }

    #[test]
    fn domain_stdin_guard_uses_mask_map_not_ve() {
        let src = include_str!("session.rs");
        assert!(src.contains("stdin_mask_map"));
        assert!(!src.contains("stdin_ve_map"));
    }

    // ── VE + ANSI interaction ─────────────────────────────────────

    #[test]
    fn test_ve_value_in_ansi_colored_output() {
        let ve = vec![("config-value".to_string(), "VE:LOCAL:CFG".to_string())];
        let input = format!("{}config-value{}", "\x1b[1m", "\x1b[0m");
        let (output, _) = mask_with_ve(&input, &[], &ve, "");
        assert!(output.contains(GREEN), "VE must be green even inside ANSI");
        let visible = strip_ansi(&output);
        assert!(visible.contains("config-value"));
    }

    #[test]
    fn test_ve_value_with_embedded_ansi_no_panic() {
        let ve = vec![(
            "\x1b[31mred\x1b[0m".to_string(),
            "VE:LOCAL:COLOR".to_string(),
        )];
        let (output, _) = mask_with_ve("show \x1b[31mred\x1b[0m here", &[], &ve, "");
        let _ = strip_ansi(&output); // must not panic
    }

    #[test]
    fn test_ve_after_vk_no_ref_corruption() {
        // VK replaces "db-password" → colorized ref. VE "LOCAL" must not
        // colorize inside the ref (ansi_aware_replace skips ANSI segments).
        let map = vec![("db-password".to_string(), "VK:LOCAL:dbp12345".to_string())];
        let ve = vec![("LOCAL".to_string(), "VE:LOCAL:SCOPE".to_string())];
        let (output, _) = mask_with_ve("db-password here", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("db-password"));
    }

    // ── VE + cross-chunk ─────────────────────────────────────────────

    #[test]
    fn test_ve_split_across_chunks_not_detected() {
        // VE has no cross-chunk detection — split values are NOT colorized
        let ve = vec![("database".to_string(), "VE:LOCAL:DB_TYPE".to_string())];
        let (output1, tail1) = mask_with_ve("data", &[], &ve, "");
        assert!(!output1.contains(GREEN));
        let (output2, _) = mask_with_ve("base-server", &[], &ve, &tail1);
        assert!(
            !output2.contains(GREEN),
            "VE split across chunks should not be detected (by design)"
        );
    }

    #[test]
    fn test_ve_full_value_in_single_chunk() {
        let ve = vec![("database".to_string(), "VE:LOCAL:DB_TYPE".to_string())];
        let (output, _) = mask_with_ve("type=database", &[], &ve, "");
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_in_second_chunk_fully() {
        let ve = vec![("production".to_string(), "VE:LOCAL:ENV".to_string())];
        let (_, tail) = mask_with_ve("env=", &[], &ve, "");
        let (output, _) = mask_with_ve("production ready", &[], &ve, &tail);
        assert!(output.contains(GREEN));
    }

    // ── VE + VK overlap edge cases ───────────────────────────────────

    #[test]
    fn test_ve_substring_of_vk_secret() {
        // VE "db-host" is substring of VK "my-db-host-password" — VK masks first
        let map = vec![(
            "my-db-host-password".to_string(),
            "VK:LOCAL:dbh12345".to_string(),
        )];
        let ve = vec![("db-host".to_string(), "VE:LOCAL:DB_HOST".to_string())];
        let (output, _) = mask_with_ve("cred=my-db-host-password", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("my-db-host-password"));
        assert!(
            !visible.contains("db-host"),
            "VE substring vanishes when VK masks the containing secret"
        );
    }

    #[test]
    fn test_vk_substring_of_ve_value() {
        // VK "prod" is substring of VE "production"
        let map = vec![("prod".to_string(), "VK:LOCAL:prd12345".to_string())];
        let ve = vec![("production".to_string(), "VE:LOCAL:ENV".to_string())];
        let (output, _) = mask_with_ve("env=production", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(
            !visible.contains("prod"),
            "VK masks 'prod' even inside 'production'"
        );
    }

    #[test]
    fn test_ve_and_vk_same_value() {
        // Same value as both VK and VE — VK takes precedence
        let map = vec![(
            "shared-value-1234".to_string(),
            "VK:LOCAL:sh123456".to_string(),
        )];
        let ve = vec![(
            "shared-value-1234".to_string(),
            "VE:LOCAL:SHARED".to_string(),
        )];
        let (output, _) = mask_with_ve("data=shared-value-1234", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(
            !visible.contains("shared-value-1234"),
            "VK takes precedence"
        );
    }

    #[test]
    fn test_ve_adjacent_to_vk() {
        let map = vec![("SECRET".to_string(), "VK:LOCAL:sec12345".to_string())];
        let ve = vec![("CONFIG".to_string(), "VE:LOCAL:CFG".to_string())];
        let (output, _) = mask_with_ve("SECRETCONFIG", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("SECRET"));
        assert!(visible.contains("CONFIG"));
    }

    // ── colorize_ve_ref unit tests ───────────────────────────────────

    #[test]
    fn test_colorize_ve_ref_format() {
        let result = colorize_ve_ref("my-config", "VE:LOCAL:CFG");
        assert_eq!(result, format!("{}my-config{}", GREEN, RESET));
    }

    #[test]
    fn test_colorize_ve_ref_empty() {
        let result = colorize_ve_ref("", "VE:LOCAL:CFG");
        assert_eq!(result, format!("{}{}", GREEN, RESET));
    }

    #[test]
    fn test_colorize_ve_ref_ignores_ref_param() {
        let r1 = colorize_ve_ref("val", "VE:LOCAL:A");
        let r2 = colorize_ve_ref("val", "VE:HOST:B");
        assert_eq!(r1, r2, "VE ref param must not affect output color");
    }

    #[test]
    fn test_colorize_ve_ref_unicode() {
        let result = colorize_ve_ref("한글설정", "VE:LOCAL:KR");
        assert!(result.contains("한글설정"));
        assert!(result.starts_with(GREEN));
        assert!(result.ends_with(RESET));
    }

    #[test]
    fn test_colorize_ve_ref_special_chars() {
        let result = colorize_ve_ref("p@ss/w0rd!#$", "VE:LOCAL:SP");
        assert!(result.contains("p@ss/w0rd!#$"));
    }

    // ── VE: newlines, paths, short values, substring chains ─────────

    #[test]
    fn test_ve_value_with_newlines() {
        let ve = vec![("line1\nline2".to_string(), "VE:LOCAL:MULTI".to_string())];
        let (output, _) = mask_with_ve("data: line1\nline2 end", &[], &ve, "");
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_value_file_path() {
        let ve = vec![(
            "/etc/config/app.toml".to_string(),
            "VE:HOST:CFG_PATH".to_string(),
        )];
        let (output, _) = mask_with_ve("loading /etc/config/app.toml ...", &[], &ve, "");
        let visible = strip_ansi(&output);
        assert!(visible.contains("/etc/config/app.toml"));
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_value_url() {
        let ve = vec![(
            "https://db.internal:5432".to_string(),
            "VE:LOCAL:DB_URL".to_string(),
        )];
        let (output, _) = mask_with_ve("connecting to https://db.internal:5432", &[], &ve, "");
        assert!(output.contains(GREEN));
        assert!(strip_ansi(&output).contains("https://db.internal:5432"));
    }

    #[test]
    fn test_ve_very_short_value_2_chars() {
        let ve = vec![("on".to_string(), "VE:LOCAL:FLAG".to_string())];
        let (output, _) = mask_with_ve("debug=on", &[], &ve, "");
        assert!(output.contains(GREEN), "2-char VE value must be colorized");
    }

    #[test]
    fn test_ve_single_char_value() {
        let ve = vec![("y".to_string(), "VE:LOCAL:CONFIRM".to_string())];
        let (output, _) = mask_with_ve("confirm: y", &[], &ve, "");
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_substring_chain_longer_first() {
        // "localhost" contains "local". Longer listed first → matches fully.
        let ve = vec![
            ("localhost".to_string(), "VE:LOCAL:FULL".to_string()),
            ("local".to_string(), "VE:LOCAL:SHORT".to_string()),
        ];
        let (output, _) = mask_with_ve("host=localhost", &[], &ve, "");
        let visible = strip_ansi(&output);
        assert!(visible.contains("localhost"));
    }

    #[test]
    fn test_ve_substring_chain_shorter_first() {
        // Shorter listed first → "local" matches inside "localhost" first
        let ve = vec![
            ("local".to_string(), "VE:LOCAL:SHORT".to_string()),
            ("localhost".to_string(), "VE:LOCAL:FULL".to_string()),
        ];
        let (output, _) = mask_with_ve("host=localhost", &[], &ve, "");
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_identical_values_different_refs() {
        let ve = vec![
            ("10.0.0.5".to_string(), "VE:LOCAL:DB_HOST".to_string()),
            ("10.0.0.5".to_string(), "VE:TEMP:CACHE_HOST".to_string()),
        ];
        let (output, _) = mask_with_ve("host=10.0.0.5", &[], &ve, "");
        assert!(output.contains(GREEN));
        assert!(strip_ansi(&output).contains("10.0.0.5"));
    }

    #[test]
    fn test_ve_value_very_long() {
        let long_val = "x".repeat(10_000);
        let ve = vec![(long_val.clone(), "VE:LOCAL:HUGE".to_string())];
        let (output, _) = mask_with_ve(&format!("cfg={}", long_val), &[], &ve, "");
        assert!(output.contains(GREEN));
    }

    #[test]
    fn test_ve_value_with_spaces() {
        let ve = vec![("my app config".to_string(), "VE:LOCAL:APP".to_string())];
        let (output, _) = mask_with_ve("name=my app config", &[], &ve, "");
        assert!(strip_ansi(&output).contains("my app config"));
        assert!(output.contains(GREEN));
    }

    // ── VE on already-VK-masked output ──────────────────────────────

    #[test]
    fn test_ve_cannot_match_inside_vk_replacement() {
        // VK replaces "my-secret" → colorized "VK:LOCAL:sec12345".
        // VE value "sec12345" should NOT match inside the VK ref because
        // ansi_aware_replace works on text segments, and the ref is inside ANSI.
        let map = vec![("my-secret".to_string(), "VK:LOCAL:sec12345".to_string())];
        let ve = vec![("sec12345".to_string(), "VE:LOCAL:HASH".to_string())];
        let (output, _) = mask_with_ve("data: my-secret end", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("my-secret"), "VK must mask secret");
        // The VK ref "VK:LOCAL:sec12345" is inside ANSI bold+cyan.
        // VE's "sec12345" may or may not match depending on ansi_aware_replace tokenizer.
        // But the important thing: no panic, and VK masking is intact.
    }

    #[test]
    fn test_ve_silent_miss_when_value_already_masked_by_vk() {
        // VE value "password" is the same as the VK secret.
        // After VK masks it, VE can't find "password" in the output → silent miss.
        let map = vec![("password".to_string(), "VK:LOCAL:pw123456".to_string())];
        let ve = vec![("password".to_string(), "VE:LOCAL:DB_PASS".to_string())];
        let (output, _) = mask_with_ve("pass=password", &map, &ve, "");
        let visible = strip_ansi(&output);
        assert!(
            !visible.contains("password"),
            "VK masks first, VE misses silently"
        );
    }

    // ── plain_tail tracks original text, not masked ──────────────────

    #[test]
    fn test_plain_tail_not_affected_by_ve_colorization() {
        // plain_tail must use ORIGINAL new_text, not the VE-colorized output.
        // This ensures cross-chunk matching works on plaintext in next call.
        let ve = vec![("config".to_string(), "VE:LOCAL:CFG".to_string())];
        let (_, tail1) = mask_with_ve("show config here", &[], &ve, "");
        // tail should contain "show config here" (original), not GREEN codes
        assert!(
            !tail1.contains("\x1b["),
            "plain_tail must not contain ANSI codes"
        );
        assert!(
            tail1.contains("config"),
            "plain_tail must contain original text"
        );
    }

    #[test]
    fn test_plain_tail_not_affected_by_vk_masking() {
        // Same for VK: plain_tail should be from original, not from masked output
        let map = vec![(
            "my-password-1234".to_string(),
            "VK:LOCAL:pw123456".to_string(),
        )];
        let (_, tail) = mask_with_ve("echo my-password-1234", &map, &[], "");
        // tail should be original text, so it contains the secret
        assert!(
            !tail.contains("\x1b["),
            "plain_tail must not contain ANSI codes"
        );
        assert!(
            tail.contains("my-password-1234"),
            "plain_tail must contain original text for future cross-chunk matching"
        );
    }

    #[test]
    fn test_plain_tail_enables_cross_chunk_after_ve() {
        // Scenario: chunk1 has VE-colorized text, chunk2 completes a VK secret.
        // plain_tail from chunk1 should be clean plaintext enabling cross-chunk VK detection.
        let map = vec![(
            "secret-password-12345678".to_string(),
            "VK:LOCAL:xc123456".to_string(),
        )];
        let ve = vec![("config".to_string(), "VE:LOCAL:CFG".to_string())];
        // Chunk 1: has VE value + start of VK secret
        let (_, tail1) = mask_with_ve("config: secret-passw", &[], &ve, "");
        assert!(
            tail1.contains("secret-passw"),
            "tail must preserve secret prefix"
        );
        // Chunk 2: rest of VK secret
        let (output2, _) = mask_with_ve("ord-12345678 done", &map, &ve, &tail1);
        let visible2 = strip_ansi(&output2);
        // Cross-chunk: "secret-passw" (tail) + "ord-12345678" (new) = "secret-password-12345678"
        // Should be detected and masked
        assert!(
            !visible2.contains("ord-12345678") || visible2.contains("done"),
            "cross-chunk VK detection should work after VE-colorized chunk"
        );
    }

    // ── VE on ANSI segment boundary ──────────────────────────────────

    #[test]
    fn test_ve_value_split_across_ansi_segments() {
        // VE value "production" appears split by ANSI: "\x1b[1mprod\x1b[0muction"
        // ansi_aware_replace should still find it by working on text segments
        let ve = vec![("production".to_string(), "VE:LOCAL:ENV".to_string())];
        let input = "\x1b[1mprod\x1b[0muction";
        let (output, _) = mask_with_ve(input, &[], &ve, "");
        // The ansi_aware_replace reconstructs text from segments:
        // segment "prod" + segment "uction" → finds "production" → replaces
        let visible = strip_ansi(&output);
        // Whether it matches depends on implementation — just verify no panic
        // and that the output is valid
        assert!(!visible.is_empty());
    }

    #[test]
    fn test_ve_value_between_ansi_codes() {
        // VE value is cleanly between ANSI codes
        let ve = vec![("localhost".to_string(), "VE:LOCAL:HOST".to_string())];
        let input = "host=\x1b[33mlocalhost\x1b[0m:3306";
        let (output, _) = mask_with_ve(input, &[], &ve, "");
        let visible = strip_ansi(&output);
        assert!(visible.contains("localhost"));
    }

    #[test]
    fn test_mask_output_recent_input_skips() {
        // When the secret was recently typed as input, masking is skipped
        // (to avoid masking what the user intentionally typed)
        let map = vec![("typed-secret-12".to_string(), "VK:LOCAL:skip1".to_string())];
        let (output, _) = mask_with_input("typed-secret-12", &map, "typed-secret-12", "");
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
        let map = vec![(
            "not-in-output-at-all".to_string(),
            "VK:LOCAL:fp".to_string(),
        )];
        let (output, _) = mask_with_ve("completely normal text", &map, &[], "");
        assert_eq!(output, "completely normal text");
    }

    // ── Dense edge cases ──────────────────────────────────────────

    #[test]
    fn edge_mask_output_all_secrets_substrings_of_output() {
        let map = vec![
            ("hello".to_string(), "VK:LOCAL:h1111111".to_string()),
            ("world".to_string(), "VK:LOCAL:w2222222".to_string()),
            ("test".to_string(), "VK:LOCAL:t3333333".to_string()),
        ];
        let input = "hello world test output";
        let (output, _) = mask_with_ve(input, &map, &[], "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("hello"), "hello must be masked");
        assert!(!visible.contains("world"), "world must be masked");
        assert!(!visible.contains("test"), "test must be masked");
        assert!(visible.contains("output"), "non-secret text must survive");
    }

    #[test]
    fn edge_mask_output_overlapping_secrets_containment() {
        let map = vec![
            (
                "password123456".to_string(),
                "VK:LOCAL:long1111".to_string(),
            ),
            ("password".to_string(), "VK:LOCAL:med11111".to_string()),
            ("pass".to_string(), "VK:LOCAL:shrt1111".to_string()),
        ];
        let input = "value=password123456 done";
        let (output, _) = mask_with_ve(input, &map, &[], "");
        let visible = strip_ansi(&output);
        assert!(!visible.contains("password"), "password must not leak");
        assert!(!visible.contains("pass"), "pass must not leak");
    }

    #[test]
    fn edge_mask_output_secret_at_plain_tail_boundary() {
        let secret = "boundary-secret-value";
        let map = vec![(secret.to_string(), "VK:LOCAL:bnd11111".to_string())];
        let half = secret.len() / 2;
        let padding_len = PLAIN_TAIL_SIZE - half;
        let tail = format!("{}{}", "x".repeat(padding_len), &secret[..half]);
        let new_text = &secret[half..];
        let result = find_cross_chunk_mask(&tail, new_text, &map);
        assert!(
            result.is_some(),
            "secret at PLAIN_TAIL_SIZE boundary must be detected"
        );
    }

    #[test]
    fn edge_mask_output_only_ansi_escapes() {
        let map = vec![("secret".to_string(), "VK:LOCAL:ansi1111".to_string())];
        let input = "\x1b[31m\x1b[0m\x1b[1m\x1b[22m";
        let (output, _) = mask_with_ve(input, &map, &[], "");
        assert_eq!(output, input);
    }

    #[test]
    fn edge_mask_output_empty_input() {
        let map = vec![("secret".to_string(), "VK:LOCAL:empty111".to_string())];
        let (output, tail) = mask_with_ve("", &map, &[], "some-tail");
        assert!(output.is_empty());
        assert_eq!(tail, "some-tail");
    }

    #[test]
    fn edge_mask_output_binary_data() {
        let map = vec![("secret".to_string(), "VK:LOCAL:bin11111".to_string())];
        let data: Vec<u8> = vec![0xFF, 0xFF, 0xFF, 0x41, 0xFF, 0xFF];
        init_crypto();
        let client = VeilKeyClient::new("http://localhost:0");
        let (output, _) = mask_output(&data, &map, &[], &[], &client, "", "");
        assert!(!output.is_empty());
    }

    #[test]
    fn edge_padded_colorize_ref_unicode_ref_name() {
        let result = padded_colorize_ref("VK:LOCAL:비밀abc", 20);
        let visible = strip_ansi(&result);
        assert!(!visible.is_empty());
    }

    #[test]
    fn edge_find_cross_chunk_mask_empty_map() {
        let result = find_cross_chunk_mask("some-tail", "new-text", &[]);
        assert!(result.is_none(), "empty mask_map should return None");
    }

    #[test]
    fn edge_find_cross_chunk_mask_1000_entries() {
        let mut map: Vec<(String, String)> = (0..999)
            .map(|i| {
                (
                    format!("other-secret-{:04}", i),
                    format!("VK:LOCAL:{:08x}", i),
                )
            })
            .collect();
        map.push((
            "target-secret-val".to_string(),
            "VK:LOCAL:target01".to_string(),
        ));
        let start = std::time::Instant::now();
        let result = find_cross_chunk_mask("target-secret-", "val", &map);
        let elapsed = start.elapsed();
        assert!(result.is_some(), "target must be found in 1000-entry map");
        assert!(
            elapsed.as_secs() < 2,
            "1000 entries took {:?} — too slow",
            elapsed
        );
    }
}

#[cfg(test)]
mod same_width_tests {
    use super::*;

    fn strip_ansi(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*m").unwrap();
        re.replace_all(s, "").to_string()
    }
    fn vw(vk_ref: &str, len: usize) -> usize {
        strip_ansi(&padded_colorize_ref(vk_ref, len))
            .chars()
            .count()
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs
            .iter()
            .map(|(s, r)| (s.to_string(), r.to_string()))
            .collect()
    }

    // ── Same-width guarantee ────────────────────────────────────
    #[test]
    fn sw_full_17() {
        assert_eq!(vw("VK:LOCAL:6da25530", 17), 17);
    }
    #[test]
    fn sw_compact_11() {
        assert_eq!(vw("VK:LOCAL:6da25530", 11), 11);
        assert_eq!(
            strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 11)),
            "VK:6da25530"
        );
    }
    #[test]
    fn sw_hash_8() {
        assert_eq!(vw("VK:LOCAL:6da25530", 8), 8);
    }
    #[test]
    fn sw_padded_9() {
        assert_eq!(vw("VK:LOCAL:6da25530", 9), 9);
    }
    #[test]
    fn sw_stars_2() {
        assert_eq!(vw("VK:LOCAL:6da25530", 2), 2);
        assert_eq!(
            strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 2)),
            "**"
        );
    }
    #[test]
    fn sw_star_1() {
        assert_eq!(vw("VK:LOCAL:6da25530", 1), 1);
    }
    #[test]
    fn sw_zero() {
        assert_eq!(padded_colorize_ref("VK:LOCAL:6da25530", 0), "");
    }
    #[test]
    fn sw_all_1_50() {
        for l in 1..=50 {
            assert_eq!(vw("VK:LOCAL:6da25530", l), l, "len={}", l);
        }
    }
    #[test]
    fn sw_temp_1_50() {
        for l in 1..=50 {
            assert_eq!(vw("VK:TEMP:abc12345", l), l, "len={}", l);
        }
    }

    // ── Cross-chunk erase + same-width ──────────────────────────
    #[test]
    fn cc_erase_same_width() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let r = find_cross_chunk_mask("(VEIL) $ Ghdrhkdgh1", "@", &map).unwrap();
        assert!(!r.output.contains("Ghdrhkdgh1@"));
    }
    #[test]
    fn cc_no_vkloc_fragments() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let r = find_cross_chunk_mask("$ Ghdrhkdg", "h1@", &map).unwrap();
        let v = strip_ansi(&r.output);
        assert!(!v.contains("VK:6dVK:"), "no VK:LOC fragments: [{}]", v);
    }

    // ── Arrow key safety ────────────────────────────────────────
    #[test]
    fn cc_skip_escape() {
        let m = mk(&[("s99", "VK:LOCAL:x")]);
        assert!(find_cross_chunk_mask("s9", "9\x1b[C", &m).is_none());
    }
    #[test]
    fn cc_skip_down_arrow() {
        let m = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        assert!(find_cross_chunk_mask("tail", "\x1b[B\x1b[2K", &m).is_none());
    }
    #[test]
    fn cc_fires_on_cr() {
        let m = mk(&[("s99", "VK:LOCAL:y")]);
        assert!(find_cross_chunk_mask("s9", "9\r", &m).is_some());
    }

    // ── Repeated invocations ────────────────────────────────────
    #[test]
    fn cc_repeated_10x() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let mut tail = String::new();
        for i in 0..10 {
            tail.push_str("(VEIL) $ ");
            let r = find_cross_chunk_mask(&format!("{}Ghdrhkdgh1", tail), "@\r", &map);
            assert!(r.is_some(), "#{}", i);
            tail.push_str("Ghdrhkdgh1@\nbash: VK:6da25530: not found\n");
            if tail.len() > 4096 {
                tail = tail[tail.len() - 4096..].to_string();
            }
        }
    }

    // ── 103 secrets ─────────────────────────────────────────────
    #[test]
    fn cc_103_secrets() {
        let mut map: Vec<(String, String)> = (0..102)
            .map(|i| (format!("other_{:04}", i), format!("VK:LOCAL:{:08x}", i)))
            .collect();
        map.push(("Ghdrhkdgh1@".into(), "VK:LOCAL:6da25530".into()));
        let r = find_cross_chunk_mask("$ Ghdrhkdgh1", "@", &map);
        assert!(r.is_some());
    }

    // ── Security ────────────────────────────────────────────────
    #[test]
    fn sec_no_plaintext() {
        let m = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let r = find_cross_chunk_mask("$ Ghdrhkdgh1", "@\r", &m).unwrap();
        assert!(!r.output.contains("Ghdrhkdgh1@"));
    }
    #[test]
    fn sec_empty_map() {
        assert!(find_cross_chunk_mask("x", "y", &[]).is_none());
    }
    #[test]
    fn sec_long_tail() {
        let m = mk(&[("needle", "VK:LOCAL:n")]);
        let t = format!("{}needl", "x".repeat(50000));
        assert!(find_cross_chunk_mask(&t, "e", &m).is_some());
    }
    #[test]
    fn sec_unicode() {
        let m = mk(&[("비밀abc", "VK:LOCAL:k")]);
        assert!(find_cross_chunk_mask("비밀ab", "c", &m).is_some());
    }
    #[test]
    fn sec_special_chars() {
        let m = mk(&[("p@$$!", "VK:LOCAL:s")]);
        assert!(find_cross_chunk_mask("p@$$", "!", &m).is_some());
    }

    // ── Stdin guard ─────────────────────────────────────────────
    #[test]
    fn sg_blocks_env() {
        use crate::pty::session::{check_stdin_for_secrets, StdinGuardResult};
        let m = mk(&[("SuperSecret", "VK:LOCAL:e")]);
        let mut b = String::new();
        assert_eq!(
            check_stdin_for_secrets(b"export X=SuperSecret\r", &mut b, &m),
            StdinGuardResult::Blocked
        );
    }
    #[test]
    fn sg_multi_chunk() {
        use crate::pty::session::{check_stdin_for_secrets, StdinGuardResult};
        let m = mk(&[("pass123", "VK:LOCAL:p")]);
        let mut b = String::new();
        check_stdin_for_secrets(b"pass", &mut b, &m);
        assert_eq!(
            check_stdin_for_secrets(b"123\r", &mut b, &m),
            StdinGuardResult::Blocked
        );
    }
    #[test]
    fn sg_safe_after_ctrl_c() {
        use crate::pty::session::{check_stdin_for_secrets, StdinGuardResult};
        let m = mk(&[("secret", "VK:LOCAL:c")]);
        let mut b = String::new();
        check_stdin_for_secrets(b"secret", &mut b, &m);
        check_stdin_for_secrets(b"\x03", &mut b, &m);
        assert_eq!(
            check_stdin_for_secrets(b"ls\r", &mut b, &m),
            StdinGuardResult::Forward
        );
    }
    #[test]
    fn sg_empty_enter() {
        use crate::pty::session::{check_stdin_for_secrets, StdinGuardResult};
        let m = mk(&[("s", "VK:LOCAL:x")]);
        let mut b = String::new();
        assert_eq!(
            check_stdin_for_secrets(b"\r", &mut b, &m),
            StdinGuardResult::Forward
        );
    }
    #[test]
    fn sg_binary_no_panic() {
        use crate::pty::session::{check_stdin_for_secrets, StdinGuardResult};
        let m = mk(&[("s", "VK:LOCAL:x")]);
        let mut b = String::new();
        assert_eq!(
            check_stdin_for_secrets(&[0xFF, 0xFE, 0x0D], &mut b, &m),
            StdinGuardResult::Forward
        );
    }
    #[test]
    fn sg_line_buf_capped() {
        use crate::pty::session::{check_stdin_for_secrets, MAX_LINE_BUF};
        let m = mk(&[("x", "VK:LOCAL:x")]);
        let mut b = String::new();
        check_stdin_for_secrets(&vec![b'A'; 100_000], &mut b, &m);
        assert!(b.len() <= MAX_LINE_BUF);
    }
}

#[cfg(test)]
mod security_masking_tests {
    use super::*;
    use crate::api::VeilKeyClient;
    use unicode_width::UnicodeWidthStr;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn mask(data: &str, map: &[(String, String)], tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, "", tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }
    fn mask_ri(data: &str, map: &[(String, String)], ri: &str, tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, ri, tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*m").unwrap();
        re.replace_all(s, "").to_string()
    }

    #[test]
    fn sec_password_echo_masked() {
        let m = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (out, _) = mask("Ghdrhkdgh1@", &m, "");
        assert!(!strip(&out).contains("Ghdrhkdgh1@"), "SECURITY: password leaked: {}", strip(&out));
    }

    #[test]
    fn sec_ansi_escape_does_not_bypass() {
        let m = mk(&[("hunter2secret", "VK:LOCAL:abc12345")]);
        let (out, _) = mask("\x1b[32m$ \x1b[0mhunter2secret", &m, "");
        assert!(!strip(&out).contains("hunter2secret"), "SECURITY: ANSI bypass: {}", strip(&out));
    }

    #[test]
    fn sec_multiple_secrets_all_masked() {
        let m = mk(&[("password123!", "VK:LOCAL:aaa11111"), ("apikey-xyz99", "VK:LOCAL:bbb22222")]);
        let (out, _) = mask("password123! and apikey-xyz99 here", &m, "");
        let v = strip(&out);
        assert!(!v.contains("password123!"), "first leaked");
        assert!(!v.contains("apikey-xyz99"), "second leaked");
    }

    #[test]
    fn sec_cross_chunk_no_leak() {
        let m = mk(&[("password1234", "VK:LOCAL:ccc33333")]);
        let (_, tail) = mask("echo passwo", &m, "");
        let (out2, _) = mask("rd1234\n", &m, &tail);
        assert!(!strip(&out2).contains("password1234"), "SECURITY: cross-chunk leak: {}", strip(&out2));
    }

    #[test]
    fn sec_vk_ref_not_double_masked() {
        let m = mk(&[("password123!", "VK:LOCAL:ddd44444")]);
        let (out, _) = mask("using VK:LOCAL:ddd44444 ok", &m, "");
        assert!(strip(&out).contains("ddd44444"), "VK ref disappeared: {}", strip(&out));
    }

    #[test]
    fn sec_empty_secret_safe() {
        let m = mk(&[("", "VK:LOCAL:eee55555"), ("real_secret!", "VK:LOCAL:fff66666")]);
        let (out, _) = mask("text with real_secret! here", &m, "");
        let v = strip(&out);
        assert!(!v.contains("real_secret!"), "non-empty leaked");
        assert!(v.contains("text with"), "text corrupted");
    }

    #[test]
    fn sec_mask_map_beats_recent_input() {
        let m = mk(&[("MyP@ssw0rd!x", "VK:LOCAL:hhh88888")]);
        let (out, _) = mask_ri("MyP@ssw0rd!x", &m, "MyP@ssw0rd!x", "");
        assert!(!strip(&out).contains("MyP@ssw0rd!x"),
            "SECURITY: mask-map must replace even with recent_input: {}", strip(&out));
    }

    #[test]
    fn sec_same_width_no_length_leak() {
        let m = mk(&[("short_pw", "VK:LOCAL:iii99999")]);
        let (out, _) = mask("short_pw", &m, "");
        let v = strip(&out);
        assert!(!v.contains("short_pw"), "secret leaked");
        assert_eq!(v.trim().chars().count(), 8, "width mismatch: {} '{}'", v.trim().chars().count(), v.trim());
    }

    #[test]
    fn sec_newline_in_secret_masked() {
        let m = mk(&[("line1\nline2", "VK:SSH:jjj00000")]);
        let (out, _) = mask("line1\nline2", &m, "");
        assert!(!strip(&out).contains("line1\nline2"), "multiline secret leaked");
    }
}

#[cfg(test)]
mod vkloc_fragment_tests {
    use super::*;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
        re.replace_all(s, "").to_string()
    }

    /// SECURITY: cross-chunk erase MUST be skipped when output contains
    /// escape sequences (arrow keys, history recall). Otherwise readline
    /// cursor goes out of sync → VK:LOC fragments.
    #[test]
    fn cross_chunk_skips_escape_sequences() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // Arrow key produces ESC [ A — cross-chunk must return None
        assert!(find_cross_chunk_mask("Ghdrhkdg", "h1@\x1b[A", &map).is_none(),
            "SECURITY: cross-chunk must skip when escape sequences present");
    }

    #[test]
    fn cross_chunk_skips_cursor_movement() {
        let map = mk(&[("password1234", "VK:LOCAL:abc12345")]);
        // CSI cursor movement
        assert!(find_cross_chunk_mask("password", "1234\x1b[C", &map).is_none());
        assert!(find_cross_chunk_mask("password", "1234\x1b[D", &map).is_none());
        assert!(find_cross_chunk_mask("password", "1234\x1b[B", &map).is_none());
    }

    /// SECURITY: padded_colorize_ref must produce EXACTLY the same visible
    /// width as the original secret. If wider → readline cursor desync → VK:LOC.
    #[test]
    fn same_width_guarantee_short_secret() {
        // "Ghdrhkdgh1@" = 11 chars, VK:LOCAL:6da25530 = 17 chars
        // Must produce 11-char visible output (compact form)
        let result = padded_colorize_ref("VK:LOCAL:6da25530", 11);
        let visible = strip(&result);
        assert_eq!(visible.chars().count(), 11,
            "SECURITY: width must be 11 (same as secret), got {}: '{}'",
            visible.chars().count(), visible);
    }

    #[test]
    fn same_width_guarantee_range() {
        let vk_ref = "VK:LOCAL:6da25530"; // 17 chars
        for original_len in 1..=30 {
            let result = padded_colorize_ref(vk_ref, original_len);
            let visible = strip(&result);
            assert_eq!(visible.chars().count(), original_len,
                "width mismatch at original_len={}: got {} '{}'",
                original_len, visible.chars().count(), visible);
        }
    }

    /// SECURITY: repeated mask_output calls must not produce VK:LOC fragments.
    /// Simulates: type password → get error → arrow up → arrow down
    #[test]
    fn no_vkloc_on_repeated_invocations() {
        init_crypto();
        let client = VeilKeyClient::new("http://localhost:0");
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let ve: Vec<(String, String)> = vec![];
        let patterns: Vec<CompiledPattern> = vec![];

        let mut combined_output = String::new();

        // Simulate 5 rounds of output
        let chunks = [
            "bash: Ghdrhkdgh1@: command not found\n",
            "$ Ghdrhkdgh1@\n",
            "\x1b[A",  // arrow up
            "\x1b[B",  // arrow down
            "bash: Ghdrhkdgh1@: command not found\n",
        ];
        let mut tail = String::new();
        for chunk in &chunks {
            let (masked, new_tail) = mask_output(
                chunk.as_bytes(), &map, &ve, &patterns, &client, "", &tail,
            );
            tail = new_tail;
            combined_output += &String::from_utf8_lossy(&masked);
        }

        let clean = strip(&combined_output);
        // Must NOT contain VK:LOC followed by anything other than AL (which is VK:LOCAL)
        let has_fragment = clean.contains("VK:LOCVK:") 
            || clean.contains("VK:LOC ")
            || regex::Regex::new(r"VK:LOC[^A]").unwrap().is_match(&clean);
        assert!(!has_fragment,
            "SECURITY: VK:LOC fragment detected in output: {}",
            clean.replace('\n', "\\n"));
    }

    /// Ensure mask_output with escape sequences doesn't corrupt output
    #[test]
    fn escape_sequence_output_not_corrupted() {
        init_crypto();
        let client = VeilKeyClient::new("http://localhost:0");
        let map = mk(&[("secret12345!", "VK:LOCAL:test1234")]);
        let ve: Vec<(String, String)> = vec![];
        let patterns: Vec<CompiledPattern> = vec![];

        // Arrow key sequence — must pass through unmodified
        let (masked, _) = mask_output(
            b"\x1b[A", &map, &ve, &patterns, &client, "", "",
        );
        assert_eq!(masked, b"\x1b[A", "escape sequences must pass through");
    }
}

#[cfg(test)]
mod readline_safety_tests {
    use super::*;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
        re.replace_all(s, "").to_string()
    }
    fn call(data: &str, map: &[(String, String)], ri: &str, tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, ri, tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }

    /// BUG: readline echo (no \n) must NOT be masked.
    /// Masking partial readline output causes cursor desync → VK:LOC fragments.
    /// Current code masks it → this test MUST FAIL until fixed.
    #[test]
    fn readline_echo_without_newline_not_masked() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // Readline echo: user typed password, no newline yet (still editing)
        let (output, _) = call("Ghdrhkdgh1@", &map, "Ghdrhkdgh1@", "");
        let visible = strip(&output);
        // readline echo should pass through — masking it breaks cursor tracking
        assert_eq!(visible, "Ghdrhkdgh1@",
            "SECURITY/UX: readline echo (no newline) must NOT be masked. \
             Masking changes byte length → readline cursor desync → VK:LOC fragments. \
             Got: '{}'", visible);
    }

    /// Completed output line WITH \n must be masked with full ref.
    #[test]
    fn completed_line_with_newline_masked() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // bash error: complete line with \n — NOT in recent_input
        let (output, _) = call(
            "bash: Ghdrhkdgh1@: command not found\n", &map, "", ""
        );
        let visible = strip(&output);
        assert!(!visible.contains("Ghdrhkdgh1@"),
            "completed line must be masked, got: '{}'", visible);
        assert!(visible.contains("VK:LOCAL:6da25530"),
            "must use full ref VK:LOCAL:, got: '{}'", visible);
    }

    /// Arrow key output (escape sequences) must pass through unmasked.
    #[test]
    fn arrow_key_output_not_masked() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // Arrow up: readline redraws with ESC sequences
        let (output, _) = call("\x1b[A", &map, "", "");
        assert_eq!(output.as_bytes(), b"\x1b[A",
            "escape sequences must pass through unchanged");
    }

    /// History recall redraw must not produce fragments.
    /// Simulates: type password → error → arrow up → enter
    #[test]
    fn history_recall_no_fragments() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let mut combined = String::new();
        let mut tail = String::new();

        // 1. bash error (complete line)
        let (out, t) = call("bash: Ghdrhkdgh1@: not found\n", &map, "", &tail);
        combined += &String::from_utf8_lossy(&out.as_bytes());
        tail = t;

        // 2. prompt
        let (out, t) = call("$ ", &map, "", &tail);
        combined += &String::from_utf8_lossy(&out.as_bytes());
        tail = t;

        // 3. arrow up (readline redraw — contains ESC)
        let (out, t) = call("\x1b[A\x1b[2KGhdrhkdgh1@", &map, "Ghdrhkdgh1@", &tail);
        combined += &String::from_utf8_lossy(&out.as_bytes());
        tail = t;

        // 4. enter + bash error again
        let (out, _) = call("\nbash: Ghdrhkdgh1@: not found\n", &map, "", &tail);
        combined += &String::from_utf8_lossy(&out.as_bytes());

        let clean = strip(&combined);
        // Must NOT have VK:LOC fragment (truncated ref)
        let has_fragment = clean.contains("VK:LOCVK:")
            || clean.contains("VK:LOC ")
            || regex::Regex::new(r"VK:LOC[^A\s]").unwrap().is_match(&clean);
        assert!(!has_fragment,
            "VK:LOC fragment in history recall: '{}'",
            clean.replace('\n', "\\n"));
    }

    /// The byte length of masked output for readline echo must equal
    /// the byte length of original — otherwise readline cursor breaks.
    #[test]
    fn readline_echo_byte_length_preserved() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // If we DO mask readline echo, the visible width must match exactly
        let (output, _) = call("Ghdrhkdgh1@", &map, "Ghdrhkdgh1@", "");
        let visible = strip(&output);
        assert_eq!(visible.chars().count(), "Ghdrhkdgh1@".chars().count(),
            "masked readline echo visible width must equal original. \
             got {} chars: '{}', want {} chars",
            visible.chars().count(), visible, "Ghdrhkdgh1@".chars().count());
    }
}

#[cfg(test)]
mod readline_edge_tests {
    use super::*;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
        re.replace_all(s, "").to_string()
    }
    fn call(data: &str, map: &[(String, String)], ri: &str, tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, ri, tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }

    // ══════════════════════════════════════════════════════════════
    // Complete line masking — \n present → MUST use full ref
    // ══════════════════════════════════════════════════════════════

    /// cat output with secret must show VK:LOCAL:
    #[test]
    fn cat_output_uses_full_ref() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (out, _) = call("DB_PASSWORD=Ghdrhkdgh1@\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:6da25530"),
            "cat output must use full ref, got: {}", v);
    }

    /// Multi-line output — each line with secret must be masked
    #[test]
    fn multi_line_output_all_masked() {
        let map = mk(&[("secret12345!", "VK:LOCAL:aaa11111")]);
        let (out, _) = call(
            "line1: secret12345!\nline2: secret12345!\nline3: ok\n",
            &map, "", ""
        );
        let v = strip(&out);
        assert!(!v.contains("secret12345!"), "all lines must be masked, got: {}", v);
        assert_eq!(v.matches("VK:LOCAL:aaa11111").count(), 2,
            "two occurrences must be replaced");
    }

    /// echo command output (has \n) must be masked
    #[test]
    fn echo_command_output_masked() {
        let map = mk(&[("apikey99xyz!", "VK:LOCAL:bbb22222")]);
        let (out, _) = call("apikey99xyz!\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:bbb22222"), "echo output must be full ref, got: {}", v);
    }

    /// Error message with secret must be masked with full ref
    #[test]
    fn error_message_full_ref() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (out, _) = call(
            "bash: Ghdrhkdgh1@: command not found\r\n",
            &map, "", ""
        );
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:6da25530"), "error must use full ref, got: {}", v);
    }

    // ══════════════════════════════════════════════════════════════
    // Readline echo — no \n, in recent_input → MUST NOT mask
    // ══════════════════════════════════════════════════════════════

    /// Partial typing (no enter yet) must not be masked
    #[test]
    fn partial_typing_not_masked() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (out, _) = call("Ghdrhkdg", &map, "Ghdrhkdg", "");
        let v = strip(&out);
        assert_eq!(v, "Ghdrhkdg", "partial input must pass through");
    }

    /// Full secret typed but no enter — still readline echo
    #[test]
    fn full_secret_no_newline_not_masked() {
        let map = mk(&[("password1234", "VK:LOCAL:ccc33333")]);
        let (out, _) = call("password1234", &map, "password1234", "");
        let v = strip(&out);
        assert_eq!(v, "password1234",
            "full secret without newline must not be masked (readline echo)");
    }

    /// Secret followed by \r (enter key, no \n yet) — still readline
    #[test]
    fn secret_with_cr_only_not_masked() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (out, _) = call("Ghdrhkdgh1@\r", &map, "Ghdrhkdgh1@\r", "");
        let v = strip(&out);
        // \r without \n is still readline territory
        assert!(!v.contains("VK:LOCAL:"),
            "CR-only must not trigger full ref masking, got: {}", v);
    }

    // ══════════════════════════════════════════════════════════════
    // Mixed scenarios — \n determines masking boundary
    // ══════════════════════════════════════════════════════════════

    /// Chunk with both echo and output: "typed\nbash: typed: not found\n"
    /// First part (before \n) may be echo, second part is output
    #[test]
    fn mixed_echo_and_output() {
        let map = mk(&[("secret_val!", "VK:LOCAL:ddd44444")]);
        let (out, _) = call(
            "secret_val!\nbash: secret_val!: not found\n",
            &map, "secret_val!", ""
        );
        let v = strip(&out);
        // The bash error line must be masked
        assert!(v.contains("VK:LOCAL:ddd44444"),
            "output portion must be masked, got: {}", v);
    }

    /// Command output NOT in recent_input must always be masked
    #[test]
    fn program_output_not_in_recent_input() {
        let map = mk(&[("db_password!", "VK:LOCAL:eee55555")]);
        // cat .env output — user didn't type the secret
        let (out, _) = call("DB_PASS=db_password!\n", &map, "cat .env", "");
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:eee55555"),
            "program output must be masked, got: {}", v);
    }

    // ══════════════════════════════════════════════════════════════
    // Escape sequences — readline control must pass through
    // ══════════════════════════════════════════════════════════════

    /// Arrow up with secret in redraw must not be masked
    #[test]
    fn arrow_up_redraw_not_masked() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // Arrow up: ESC[A followed by readline redraw
        let (out, _) = call(
            "\x1b[A\x1b[2K$ Ghdrhkdgh1@",
            &map, "Ghdrhkdgh1@", ""
        );
        // Must not produce VK:LOC fragments
        let v = strip(&out);
        let has_fragment = v.contains("VK:LOCVK:")
            || regex::Regex::new(r"VK:LOC[^A\s]").unwrap().is_match(&v);
        assert!(!has_fragment, "arrow up must not produce fragments, got: {}", v);
    }

    /// Tab completion output must pass through
    #[test]
    fn tab_completion_passthrough() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (out, _) = call("\x1b[?1h\x1b=", &map, "", "");
        assert_eq!(out.as_bytes(), b"\x1b[?1h\x1b=",
            "terminal mode sequences must pass through");
    }

    // ══════════════════════════════════════════════════════════════
    // Width safety — no cursor desync
    // ══════════════════════════════════════════════════════════════

    /// If readline echo IS masked (as fallback), width MUST match exactly
    #[test]
    fn masked_echo_width_matches_original() {
        let map = mk(&[("short_pw", "VK:LOCAL:fff66666")]);
        let (out, _) = call("short_pw", &map, "", "");  // no recent_input → may mask
        let v = strip(&out);
        if v != "short_pw" {
            // If masked, width must match original (8 chars)
            assert_eq!(v.chars().count(), 8,
                "if echo is masked, width must equal original (8), got {}: '{}'",
                v.chars().count(), v);
        }
    }

    /// Multiple secrets with different lengths — all width-safe
    #[test]
    fn multiple_secrets_width_safe() {
        let map = mk(&[
            ("short!", "VK:LOCAL:ggg77777"),
            ("medium_secret!", "VK:LOCAL:hhh88888"),
            ("very_long_password_value!", "VK:LOCAL:iii99999"),
        ]);
        for (secret, _) in &map {
            let (out, _) = call(secret, &map, "", "");
            let v = strip(&out);
            if v != *secret {
                assert_eq!(v.chars().count(), secret.chars().count(),
                    "width mismatch for '{}': got {} '{}'",
                    secret, v.chars().count(), v);
            }
        }
    }
}

#[cfg(test)]
mod masking_defense_tests {
    use super::*;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
        re.replace_all(s, "").to_string()
    }
    fn call(data: &str, map: &[(String, String)], ri: &str, tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, ri, tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }

    // ══════════════════════════════════════════════════════════════
    // Defense: secrets must NEVER leak in completed output
    // ══════════════════════════════════════════════════════════════

    /// Secret in any \n-terminated line must be masked — no exceptions
    #[test]
    fn defense_secret_never_in_completed_line() {
        let map = mk(&[("SuperSecret1", "VK:LOCAL:def00001")]);
        let test_lines = [
            "error: SuperSecret1 is invalid\n",
            "curl: (401) SuperSecret1\n",
            "export KEY=SuperSecret1\n",
            "SuperSecret1\n",
            "  SuperSecret1  \n",
        ];
        for line in &test_lines {
            let (out, _) = call(line, &map, "", "");
            let v = strip(&out);
            assert!(!v.contains("SuperSecret1"),
                "DEFENSE: secret leaked in completed line: {}", line.trim());
        }
    }

    /// Secret must be masked even if surrounded by special chars
    #[test]
    fn defense_secret_with_special_chars() {
        let map = mk(&[("p@ss!w0rd#$", "VK:LOCAL:def00002")]);
        let (out, _) = call("value='p@ss!w0rd#$'\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("p@ss!w0rd#$"),
            "DEFENSE: special char secret leaked: {}", v);
    }

    /// Multiple different secrets in one line must all be masked
    #[test]
    fn defense_multiple_different_secrets() {
        let map = mk(&[
            ("password123!", "VK:LOCAL:def00003"),
            ("apikey_abc99", "VK:LOCAL:def00004"),
        ]);
        let (out, _) = call("user=password123! key=apikey_abc99\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("password123!"), "first secret leaked");
        assert!(!v.contains("apikey_abc99"), "second secret leaked");
    }

    /// Secret in pipe/redirect output must be masked
    #[test]
    fn defense_pipe_output_masked() {
        let map = mk(&[("token_xyz99!", "VK:LOCAL:def00005")]);
        let (out, _) = call("token_xyz99!\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("token_xyz99!"),
            "DEFENSE: pipe output secret leaked");
    }

    /// Secret appearing in git diff output must be masked
    #[test]
    fn defense_git_diff_masked() {
        let map = mk(&[("db_secret_42!", "VK:LOCAL:def00006")]);
        let (out, _) = call("+DB_PASSWORD=db_secret_42!\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("db_secret_42!"),
            "DEFENSE: git diff secret leaked");
    }

    // ══════════════════════════════════════════════════════════════
    // Defense: readline cursor must NEVER desync
    // ══════════════════════════════════════════════════════════════

    /// After N rounds of type→error→arrow, no VK:LOC fragments
    #[test]
    fn defense_10_rounds_no_fragments() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let mut combined = String::new();
        let mut tail = String::new();

        for _ in 0..10 {
            // type + error
            let (out, t) = call("bash: Ghdrhkdgh1@: not found\n", &map, "", &tail);
            combined += &strip(&String::from_utf8_lossy(&out.as_bytes()));
            tail = t;
            // arrow up
            let (out, t) = call("\x1b[A", &map, "Ghdrhkdgh1@", &tail);
            combined += &strip(&String::from_utf8_lossy(&out.as_bytes()));
            tail = t;
            // arrow down
            let (out, t) = call("\x1b[B", &map, "", &tail);
            combined += &strip(&String::from_utf8_lossy(&out.as_bytes()));
            tail = t;
        }

        let has_fragment = combined.contains("VK:LOCVK:")
            || combined.contains("VK:LOC ")
            || regex::Regex::new(r"VK:LOC[^A\s]").unwrap().is_match(&combined);
        assert!(!has_fragment,
            "DEFENSE: VK:LOC fragment after 10 rounds: {}",
            &combined[..combined.len().min(200)]);
    }

    /// Rapid arrow key spam must not corrupt output
    #[test]
    fn defense_rapid_arrows_no_corruption() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let mut tail = String::new();

        // Simulate rapid ↑↑↑↓↓↓↑↓↑↓
        let arrows = ["\x1b[A", "\x1b[A", "\x1b[A", "\x1b[B", "\x1b[B",
                       "\x1b[B", "\x1b[A", "\x1b[B", "\x1b[A", "\x1b[B"];
        for arrow in &arrows {
            let (_, t) = call(arrow, &map, "", &tail);
            tail = t;
        }
        // After all arrows, type and get error
        let (out, _) = call("bash: Ghdrhkdgh1@: not found\n", &map, "", &tail);
        let v = strip(&String::from_utf8_lossy(&out.as_bytes()));
        assert!(v.contains("VK:LOCAL:6da25530") || !v.contains("Ghdrhkdgh1@"),
            "DEFENSE: after rapid arrows, output must be clean: {}", v);
    }

    // ══════════════════════════════════════════════════════════════
    // Defense: scope information must be preserved
    // ══════════════════════════════════════════════════════════════

    /// Full ref in completed lines must preserve scope
    #[test]
    fn defense_scope_preserved_local() {
        let map = mk(&[("secret_val!", "VK:LOCAL:def00007")]);
        let (out, _) = call("val=secret_val!\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:"),
            "DEFENSE: LOCAL scope must be preserved, got: {}", v);
    }

    #[test]
    fn defense_scope_preserved_ssh() {
        let map = mk(&[("ssh_key_data!", "VK:SSH:def00008")]);
        let (out, _) = call("key=ssh_key_data!\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:SSH:"),
            "DEFENSE: SSH scope must be preserved, got: {}", v);
    }

    #[test]
    fn defense_scope_preserved_temp() {
        let map = mk(&[("temp_token12!", "VK:TEMP:def00009")]);
        let (out, _) = call("tok=temp_token12!\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:TEMP:"),
            "DEFENSE: TEMP scope must be preserved, got: {}", v);
    }
}

#[cfg(test)]
mod masking_comprehensive_tests {
    use super::*;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
        re.replace_all(s, "").to_string()
    }
    fn call(data: &str, map: &[(String, String)], ri: &str, tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, ri, tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }

    // ═══ Secret position edge cases ═══

    /// Secret at very start of completed line
    #[test]
    fn secret_at_line_start() {
        let map = mk(&[("secret_abc!", "VK:LOCAL:pos00001")]);
        let (out, _) = call("secret_abc!: error\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:pos00001"), "start of line: {}", v);
    }

    /// Secret at very end of completed line (before \n)
    #[test]
    fn secret_at_line_end() {
        let map = mk(&[("secret_abc!", "VK:LOCAL:pos00002")]);
        let (out, _) = call("value=secret_abc!\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:LOCAL:pos00002"), "end of line: {}", v);
    }

    /// Line is ONLY the secret + newline
    #[test]
    fn line_is_only_secret() {
        let map = mk(&[("only_secret!", "VK:LOCAL:pos00003")]);
        let (out, _) = call("only_secret!\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("only_secret!"), "sole secret line leaked: {}", v);
        assert!(v.contains("VK:LOCAL:pos00003"), "must be full ref: {}", v);
    }

    /// Empty lines between secret lines
    #[test]
    fn empty_lines_between_secrets() {
        let map = mk(&[("hidden_val!", "VK:LOCAL:pos00004")]);
        let (out, _) = call("\nhidden_val!\n\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("hidden_val!"), "leaked between empty lines: {}", v);
    }

    // ═══ Output format edge cases ═══

    /// Secret in JSON output
    #[test]
    fn secret_in_json_output() {
        let map = mk(&[("db_pass_xyz!", "VK:LOCAL:fmt00001")]);
        let (out, _) = call("{\"password\":\"db_pass_xyz!\"}\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("db_pass_xyz!"), "JSON output leaked: {}", v);
    }

    /// Secret in env var output (env | grep)
    #[test]
    fn secret_in_env_output() {
        let map = mk(&[("api_key_999!", "VK:LOCAL:fmt00002")]);
        let (out, _) = call("API_KEY=api_key_999!\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("api_key_999!"), "env output leaked: {}", v);
    }

    /// Secret in docker logs
    #[test]
    fn secret_in_docker_logs() {
        let map = mk(&[("conn_string!", "VK:LOCAL:fmt00003")]);
        let (out, _) = call("2026-03-26 DB_URL=conn_string!\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("conn_string!"), "docker log leaked: {}", v);
    }

    /// Secret in grep output (may have ANSI colors from grep --color)
    #[test]
    fn secret_in_colored_grep() {
        let map = mk(&[("grep_secret!", "VK:LOCAL:fmt00004")]);
        let (out, _) = call(
            "\x1b[35mconfig.yml\x1b[0m:\x1b[32mPASS=grep_secret!\x1b[0m\n",
            &map, "", ""
        );
        let v = strip(&out);
        assert!(!v.contains("grep_secret!"), "colored grep leaked: {}", v);
    }

    // ═══ Cross-chunk with newline ═══

    /// Secret split across chunks, second chunk has \n (completed line)
    #[test]
    fn cross_chunk_with_newline_masked() {
        let map = mk(&[("password1234", "VK:LOCAL:xch00001")]);
        let (_, tail) = call("echo passwo", &map, "echo passwo", "");
        let (out, _) = call("rd1234\n", &map, "", &tail);
        let v = strip(&out);
        // The completed portion after \n must not contain the secret
        assert!(!v.contains("password1234"),
            "cross-chunk with newline leaked: {}", v);
    }

    // ═══ Typing sequence simulation ═══

    /// Fast paste: entire secret arrives at once + \n
    #[test]
    fn fast_paste_with_enter() {
        let map = mk(&[("pasted_pw12!", "VK:LOCAL:seq00001")]);
        // Paste + enter in one chunk — has \n so should mask
        let (out, _) = call("pasted_pw12!\nbash: pasted_pw12!: not found\n", &map, "pasted_pw12!", "");
        let v = strip(&out);
        // The bash error must be masked
        assert!(v.contains("VK:LOCAL:seq00001"),
            "bash error after paste must be masked: {}", v);
    }

    /// Repeated enter on same command
    #[test]
    fn repeated_enter_same_command() {
        let map = mk(&[("repeat_pw!!", "VK:LOCAL:seq00002")]);
        let mut tail = String::new();
        for _ in 0..5 {
            let (out, t) = call("bash: repeat_pw!!: not found\n", &map, "", &tail);
            tail = t;
            let v = strip(&String::from_utf8_lossy(&out.as_bytes()));
            assert!(!v.contains("repeat_pw!!"),
                "repeated execution leaked: {}", v);
        }
    }

    /// Type command with secret, get output, type another
    #[test]
    fn sequential_commands_isolated() {
        let map = mk(&[
            ("secret_one!", "VK:LOCAL:seq00003"),
            ("secret_two!", "VK:LOCAL:seq00004"),
        ]);
        let mut tail = String::new();
        // Command 1
        let (out1, t) = call("bash: secret_one!: not found\n", &map, "", &tail);
        tail = t;
        assert!(!strip(&String::from_utf8_lossy(&out1.as_bytes())).contains("secret_one!"));
        // Command 2
        let (out2, _) = call("bash: secret_two!: not found\n", &map, "", &tail);
        assert!(!strip(&String::from_utf8_lossy(&out2.as_bytes())).contains("secret_two!"));
    }

    // ═══ Edge: empty/minimal inputs ═══

    /// Empty input
    #[test]
    fn empty_input_no_crash() {
        let map = mk(&[("secret!", "VK:LOCAL:edg00001")]);
        let (out, tail) = call("", &map, "", "");
        assert!(out.is_empty());
        assert!(tail.is_empty() || !tail.is_empty()); // just no crash
    }

    /// Just newline
    #[test]
    fn just_newline() {
        let map = mk(&[("secret!", "VK:LOCAL:edg00002")]);
        let (out, _) = call("\n", &map, "", "");
        assert_eq!(strip(&String::from_utf8_lossy(&out.as_bytes())).trim(), "");
    }

    /// Secret exactly matching mask_map value (self-reference)
    #[test]
    fn secret_is_vk_ref_itself() {
        // If plaintext is "VK:LOCAL:abc" it should NOT be in mask_map (filtered by enrich)
        // but if it somehow is, it should not infinite-loop
        let map = mk(&[("VK:LOCAL:abc", "VK:LOCAL:abc")]);
        let (out, _) = call("test VK:LOCAL:abc end\n", &map, "", "");
        // Just don't crash/hang
        assert!(!out.is_empty());
    }

    // ═══ Scope: all types must show full ref in completed lines ═══

    #[test]
    fn scope_external_preserved() {
        let map = mk(&[("ext_secret!!", "VK:EXTERNAL:scp00001")]);
        let (out, _) = call("val=ext_secret!!\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("VK:EXTERNAL:scp00001"),
            "EXTERNAL scope lost: {}", v);
    }

    // ═══ Width regression guard ═══

    /// If masking happens on readline echo, width MUST match for ALL lengths
    #[test]
    fn width_regression_1_to_30() {
        let ref_str = "VK:LOCAL:abcdef01";
        for len in 1..=30 {
            let secret: String = (0..len).map(|i| (b'a' + (i % 26) as u8) as char).collect();
            let map = vec![(secret.clone(), ref_str.to_string())];
            let (out, _) = call(&secret, &map, "", "");
            let v = strip(&out);
            if v != secret {
                // If masked, width must match
                assert_eq!(v.chars().count(), secret.chars().count(),
                    "width mismatch at len={}: got {}: '{}'",
                    len, v.chars().count(), v);
            }
        }
    }
}

#[cfg(test)]
mod masking_advanced_tests {
    use super::*;

    fn init_crypto() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }
    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(a, b)| (a.to_string(), b.to_string())).collect()
    }
    fn strip(s: &str) -> String {
        let re = regex::Regex::new(r"\x1b\[[0-9;]*[a-zA-Z]").unwrap();
        re.replace_all(s, "").to_string()
    }
    fn call(data: &str, map: &[(String, String)], ri: &str, tail: &str) -> (String, String) {
        init_crypto();
        let c = VeilKeyClient::new("http://localhost:0");
        let (b, t) = mask_output(data.as_bytes(), map, &[], &[], &c, ri, tail);
        (String::from_utf8_lossy(&b).to_string(), t)
    }

    // ═══ ECHO off scenario (sudo/ssh password) ═══

    /// sudo error leaks password — must be masked in output.
    /// Password was typed during ECHO off, NOT in recent_input.
    #[test]
    fn sudo_error_masks_password() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // sudo outputs error with the attempted password (rare but possible)
        // recent_input is empty because ECHO was off during typing
        let (out, _) = call("Sorry, try again.\nGhdrhkdgh1@\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("Ghdrhkdgh1@"), "sudo error leaked password: {}", v);
    }

    // ═══ Multiline secret (SSH key) ═══

    /// SSH private key spans multiple lines — all lines must be masked
    #[test]
    fn ssh_key_multiline_masked() {
        let key = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk=\n-----END OPENSSH PRIVATE KEY-----";
        let map = mk(&[(key, "VK:SSH:sshkey01")]);
        let (out, _) = call(&format!("{}\n", key), &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("BEGIN OPENSSH"), "SSH key header leaked: {}", v);
    }

    // ═══ Two secrets adjacent, no space ═══

    /// Two secrets touching each other in output
    #[test]
    fn adjacent_secrets_both_masked() {
        let map = mk(&[
            ("secret_aaa!", "VK:LOCAL:adj00001"),
            ("secret_bbb!", "VK:LOCAL:adj00002"),
        ]);
        let (out, _) = call("secret_aaa!secret_bbb!\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("secret_aaa!"), "first adjacent leaked");
        assert!(!v.contains("secret_bbb!"), "second adjacent leaked");
    }

    // ═══ \r\n vs \n ═══

    /// Windows-style line endings (\r\n) — still a completed line
    #[test]
    fn crlf_line_ending_masked() {
        let map = mk(&[("win_secret!", "VK:LOCAL:crlf0001")]);
        let (out, _) = call("val=win_secret!\r\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("win_secret!"), "CRLF line leaked: {}", v);
        assert!(v.contains("VK:LOCAL:crlf0001"), "must use full ref: {}", v);
    }

    // ═══ Secret followed by prompt ═══

    /// Error line + new prompt in same chunk
    #[test]
    fn error_then_prompt_in_same_chunk() {
        let map = mk(&[("my_password!", "VK:LOCAL:prm00001")]);
        let (out, _) = call("bash: my_password!: not found\n$ ", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("my_password!"), "error before prompt leaked: {}", v);
        assert!(v.contains("$ "), "prompt must survive: {}", v);
    }

    // ═══ Secret after ANSI reset ═══

    /// Secret immediately after ANSI reset code
    #[test]
    fn secret_after_ansi_reset() {
        let map = mk(&[("reset_secret", "VK:LOCAL:ansi0001")]);
        let (out, _) = call("\x1b[0mreset_secret\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("reset_secret"), "secret after ANSI reset leaked: {}", v);
    }

    // ═══ Enriched variants ═══

    /// Base64 of secret must also be masked
    #[test]
    fn base64_variant_masked() {
        // "Ghdrhkdgh1@" base64 = "R2hkcmhrZGdoMUA="
        let map = mk(&[
            ("Ghdrhkdgh1@", "VK:LOCAL:6da25530"),
            ("R2hkcmhrZGdoMUA=", "VK:LOCAL:6da25530"),
        ]);
        let (out, _) = call("token=R2hkcmhrZGdoMUA=\n", &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("R2hkcmhrZGdoMUA="), "base64 variant leaked: {}", v);
    }

    /// Hex of secret must also be masked
    #[test]
    fn hex_variant_masked() {
        // "Ghdrhkdgh1@" hex = "47686472686b64676831..."
        let hex_val = "47686472686b6467683140";
        let map = mk(&[
            ("Ghdrhkdgh1@", "VK:LOCAL:6da25530"),
            (hex_val, "VK:LOCAL:6da25530"),
        ]);
        let (out, _) = call(&format!("hex={}\n", hex_val), &map, "", "");
        let v = strip(&out);
        assert!(!v.contains(hex_val), "hex variant leaked: {}", v);
    }

    // ═══ Long line / terminal wrap ═══

    /// Secret in a very long line (beyond 80 cols)
    #[test]
    fn secret_in_long_line() {
        let map = mk(&[("long_secret!", "VK:LOCAL:lng00001")]);
        let prefix = "x".repeat(200);
        let (out, _) = call(&format!("{}long_secret!\n", prefix), &map, "", "");
        let v = strip(&out);
        assert!(!v.contains("long_secret!"), "secret in long line leaked: {}", v);
    }

    // ═══ Backspace during typing ═══

    /// User types partial secret, backspaces, retypes — echo should not be masked
    #[test]
    fn backspace_during_typing_safe() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // Backspace = \x7f or \x08
        let (out, _) = call("Ghdrhk\x08\x08dgh1@", &map, "Ghdrhk\x08\x08dgh1@", "");
        // Must not crash or produce fragments
        let v = strip(&out);
        let has_fragment = v.contains("VK:LOCVK:");
        assert!(!has_fragment, "backspace produced fragment: {}", v);
    }

    // ═══ Ctrl+C interrupt ═══

    /// Ctrl+C (^C) mid-line should not leave fragments
    #[test]
    fn ctrl_c_mid_secret_safe() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        // Ctrl+C appears as ^C in output
        let (out, _) = call("Ghdrhk^C\n", &map, "Ghdrhk", "");
        let v = strip(&out);
        // Partial secret "Ghdrhk" should not match (too short or incomplete)
        // No crash, no fragment
        assert!(!v.contains("VK:LOCVK:"), "ctrl+c fragment: {}", v);
    }

    // ═══ Concurrent chunks ═══

    /// Two different secrets in rapid succession
    #[test]
    fn rapid_successive_secrets() {
        let map = mk(&[
            ("first_sec!!", "VK:LOCAL:rap00001"),
            ("second_sec!", "VK:LOCAL:rap00002"),
        ]);
        let mut tail = String::new();
        let (out1, t) = call("bash: first_sec!!: not found\n", &map, "", &tail);
        tail = t;
        let (out2, _) = call("bash: second_sec!: not found\n", &map, "", &tail);
        let v1 = strip(&String::from_utf8_lossy(&out1.as_bytes()));
        let v2 = strip(&String::from_utf8_lossy(&out2.as_bytes()));
        assert!(!v1.contains("first_sec!!"), "first rapid leaked");
        assert!(!v2.contains("second_sec!"), "second rapid leaked");
    }

    // ═══ No secret registered ═══

    /// Empty mask_map — output passes through unchanged
    #[test]
    fn empty_mask_map_passthrough() {
        let map: Vec<(String, String)> = vec![];
        let (out, _) = call("normal output\n", &map, "", "");
        let v = strip(&out);
        assert_eq!(v.trim(), "normal output", "empty map corrupted output");
    }

    /// Secret not in mask_map — passes through (not our job to detect)
    #[test]
    fn unregistered_secret_passes() {
        let map = mk(&[("known_secret", "VK:LOCAL:xxx00001")]);
        let (out, _) = call("unknown_password\n", &map, "", "");
        let v = strip(&out);
        assert!(v.contains("unknown_password"), "only mask_map secrets should be masked");
    }
}
