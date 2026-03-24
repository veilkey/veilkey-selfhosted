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
/// Full ref when it fits, compact "VK:hash" when shorter, hash-only for very short.
pub fn padded_colorize_ref(vk_ref: &str, original_len: usize) -> String {
    if original_len == 0 {
        return String::new();
    }
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

    // Line-clear removed: \r\x1b[2K erases the entire line including the
    // shell prompt, breaking readline redraws on arrow keys / history recall.
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
    let mut output_had_replacement = false;

    // Apply cross-chunk boundary replacements first (secret suffix leaked into new_text)
    for (leaked, replacement) in &cross_chunk_replacements {
        output = output.replacen(leaked, replacement, 1);
        output_had_replacement = true;
    }
    for (plaintext, vk_ref) in mask_map {
        if plaintext.is_empty() {
            continue;
        }
        let repl = padded_colorize_ref(vk_ref, UnicodeWidthStr::width(plaintext.as_str()));
        let (new_out, replaced) = ansi_aware_replace(&output, plaintext, &repl);
        if replaced {
            output = new_out;
            output_had_replacement = true;
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
                        output_had_replacement = true;
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
                        output_had_replacement = true;
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

    // Line-clear on lines that had secrets replaced
    if output_had_replacement {
        let mut cleared = String::new();
        for (i, line) in output.split('\n').enumerate() {
            if i > 0 {
                cleared.push('\n');
            }
            let has_our_ansi = line.contains("\x1b[1m\x1b[36m") || line.contains("\x1b[1m\x1b[31m");
            if has_our_ansi {
                cleared.push_str("\r\x1b[2K");
            }
            cleared.push_str(line);
        }
        output = cleared;
    }

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
        // ref 17 chars, secret 10 chars → truncate ref to 10
        let result = padded_colorize_ref("VK:LOCAL:6da25530", 10);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 10);
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
        // secret 3 chars → ref truncated heavily
        let result = padded_colorize_ref("VK:LOCAL:abc", 3);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 3);
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
        let result = padded_colorize_ref("VK:LOCAL:abc", 1);
        let visible = strip_ansi(&result);
        assert_eq!(visible.chars().count(), 1);
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
        assert_eq!(result.len(), input.len(), "line width must be preserved");
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
        assert_eq!(result.len(), input.len());
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
        assert_eq!(result.len(), input.len());
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
        assert_eq!(result.len(), input.len());
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
        assert_eq!(result.len(), input.len());
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
    fn test_width_preserved_across_various_ref_lengths() {
        for secret_len in [5, 10, 15, 17, 20, 30, 50, 100] {
            let secret: String = (0..secret_len)
                .map(|i| (b'a' + (i % 26) as u8) as char)
                .collect();
            let input = format!("prefix:{}:suffix", secret);
            let mask_map = vec![(secret.clone(), "VK:LOCAL:6da25530".to_string())];
            let result = simulate_mask(&input, &mask_map);
            assert_eq!(
                result.len(),
                input.len(),
                "width mismatch for secret_len={}: input=[{}] result=[{}]",
                secret_len,
                input,
                result
            );
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

    // ── padded_colorize_ref: same-width guarantee ───────────────────

    fn visible_len(vk_ref: &str, original_len: usize) -> usize {
        strip_ansi(&padded_colorize_ref(vk_ref, original_len)).chars().count()
    }

    #[test]
    fn width_exact_match_full_ref() {
        // secret 17 chars = full ref 17 chars → exact fit
        assert_eq!(visible_len("VK:LOCAL:6da25530", 17), 17);
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 17));
        assert_eq!(v, "VK:LOCAL:6da25530");
    }

    #[test]
    fn width_exact_match_longer_secret() {
        // secret 30 chars > ref 17 chars → padded to 30
        assert_eq!(visible_len("VK:LOCAL:6da25530", 30), 30);
    }

    #[test]
    fn width_exact_match_11_char_secret() {
        // secret 11 chars < ref 17 chars → compact "VK:6da25530" (11 chars)
        assert_eq!(visible_len("VK:LOCAL:6da25530", 11), 11);
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 11));
        assert_eq!(v, "VK:6da25530");
    }

    #[test]
    fn width_exact_match_7_char_secret() {
        // secret 7 chars → compact "VK:6da2" + padding? Let's check
        assert_eq!(visible_len("VK:LOCAL:6da25530", 7), 7);
    }

    #[test]
    fn width_exact_match_5_char_secret() {
        // secret 5 chars → hash only "6da25"
        assert_eq!(visible_len("VK:LOCAL:6da25530", 5), 5);
    }

    #[test]
    fn width_exact_match_3_char_secret() {
        assert_eq!(visible_len("VK:LOCAL:6da25530", 3), 3);
    }

    #[test]
    fn width_exact_match_2_char_secret() {
        // Too short → stars
        assert_eq!(visible_len("VK:LOCAL:6da25530", 2), 2);
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 2));
        assert_eq!(v, "**");
    }

    #[test]
    fn width_exact_match_1_char_secret() {
        assert_eq!(visible_len("VK:LOCAL:6da25530", 1), 1);
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 1));
        assert_eq!(v, "*");
    }

    #[test]
    fn width_zero() {
        assert_eq!(padded_colorize_ref("VK:LOCAL:6da25530", 0), "");
    }

    #[test]
    fn width_all_lengths_match() {
        // The core guarantee: for ALL lengths 1..50, visible width == original_len
        for len in 1..=50 {
            assert_eq!(
                visible_len("VK:LOCAL:6da25530", len), len,
                "width mismatch at len={}", len
            );
        }
    }

    #[test]
    fn width_temp_ref_same_guarantee() {
        for len in 1..=50 {
            assert_eq!(
                visible_len("VK:TEMP:abc12345", len), len,
                "TEMP ref width mismatch at len={}", len
            );
        }
    }

    #[test]
    fn compact_format_content_11() {
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 11));
        assert_eq!(v, "VK:6da25530", "11-char compact must drop LOCAL:");
    }

    #[test]
    fn compact_format_content_12() {
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 12));
        assert_eq!(v.len(), 12);
        assert!(v.starts_with("VK:6da25530"), "12-char should be compact + 1 space");
    }

    #[test]
    fn hash_only_format_content_8() {
        let v = strip_ansi(&padded_colorize_ref("VK:LOCAL:6da25530", 8));
        assert_eq!(v.len(), 8);
        assert!(v.starts_with("6da25530"), "8-char hash-only");
    }

    #[test]
    fn colorized_output() {
        // Full ref: colored with BOLD+CYAN
        let r = padded_colorize_ref("VK:LOCAL:6da25530", 17);
        assert!(r.contains(BOLD), "must have BOLD");
        assert!(r.contains(CYAN), "must have CYAN");
    }

    #[test]
    fn colorized_compact() {
        // Compact "VK:6da25530": also colored
        let r = padded_colorize_ref("VK:LOCAL:6da25530", 11);
        assert!(r.contains(BOLD));
        assert!(r.contains(CYAN));
    }

    #[test]
    fn colorized_hash_only() {
        // Hash only "6da25530": also colored
        let r = padded_colorize_ref("VK:LOCAL:6da25530", 8);
        assert!(r.contains(BOLD));
        assert!(r.contains(CYAN));
    }

    #[test]
    fn no_line_clear_in_output() {
        let data = b"echo Ghdrhkdgh1@\n";
        let map = vec![("Ghdrhkdgh1@".to_string(), "VK:LOCAL:6da25530".to_string())];
        let (out, _) = mask_output(data, &map, &[], &[], &VeilKeyClient::new("http://127.0.0.1:1"), "", "");
        let s = String::from_utf8_lossy(&out);
        assert!(!s.contains("\r\x1b[2K"), "line-clear must not be present");
        assert!(!s.contains("Ghdrhkdgh1@"), "secret must be masked");
    }

    #[test]
    fn masked_line_width_preserved() {
        let input = "bash: Ghdrhkdgh1@: command not found";
        let map = vec![("Ghdrhkdgh1@".to_string(), "VK:LOCAL:6da25530".to_string())];
        let (out, _) = mask_output(input.as_bytes(), &map, &[], &[], &VeilKeyClient::new("http://127.0.0.1:1"), "", "");
        let visible = strip_ansi(&String::from_utf8_lossy(&out));
        assert_eq!(
            visible.len(), input.len(),
            "line width must be preserved: got [{}]", visible
        );
    }
}
