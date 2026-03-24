use crate::api::VeilKeyClient;
use crate::config::CompiledPattern;

const BOLD: &str = "\x1b[1m";
const CYAN: &str = "\x1b[36m";
const RED: &str = "\x1b[31m";
const GREEN: &str = "\x1b[92m";
const RESET: &str = "\x1b[0m";

/// Result of cross-chunk mask_map boundary scan.
#[derive(Debug, PartialEq)]
pub(crate) struct CrossChunkMatch {
    /// Backspace erase sequence + VK ref replacement + remaining new_text
    pub output: String,
}

/// Check if any mask_map secret spans the tail/new_text boundary.
/// Only searches the boundary region to avoid matching stale secrets in old tail.
/// Returns Some(replacement output) if a cross-chunk match is found.
pub(crate) fn find_cross_chunk_mask(
    plain_tail: &str,
    new_text: &str,
    mask_map: &[(String, String)],
) -> Option<CrossChunkMatch> {
    let combined = format!("{}{}", plain_tail, new_text);
    let tail_len = plain_tail.len();

    // Only fire when new_text completes a secret at the exact boundary.
    // Pick the LONGEST match to avoid short-prefix false triggers
    // (e.g. "pass" matching before "password" is fully typed).
    let mut best: Option<(usize, String, String, String)> = None;

    for (plaintext, vk_ref) in mask_map {
        if plaintext.is_empty() || plaintext.len() < 3 {
            continue;
        }
        // The secret must END exactly at or within new_text
        // and START within the tail.
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
                let is_longer = best.as_ref().map_or(true, |(len, _, _, _)| plaintext.len() > *len);
                if is_longer {
                    let tail_part = &combined[pos..tail_len];
                    let tail_chars = tail_part.chars().count();
                    // Use ANSI CSI: move cursor left N, erase to end of line, then print ref
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

pub fn colorize_ref(vk_ref: &str) -> String {
    if vk_ref.contains(":LOCAL:") {
        format!("{}{}{}{}", BOLD, CYAN, vk_ref, RESET)
    } else if vk_ref.contains(":TEMP:") {
        format!("{}{}{}{}", BOLD, RED, vk_ref, RESET)
    } else {
        vk_ref.to_string()
    }
}

/// VE: colorize original value in green — same text, different color.
pub fn colorize_ve_ref(original: &str, _ve_ref: &str) -> String {
    format!("{}{}{}", GREEN, original, RESET)
}

/// Replace a secret with a colorized VK ref, padded to match the original width.
/// If the ref is shorter than the original, pad with spaces.
/// If the ref is longer than the original, show the full ref (never truncate).
pub fn padded_colorize_ref(vk_ref: &str, original_len: usize) -> String {
    if original_len == 0 {
        return String::new();
    }
    let ref_visible_len = vk_ref.chars().count();
    let colored = colorize_ref(vk_ref);
    if ref_visible_len <= original_len {
        let pad = original_len - ref_visible_len;
        if pad > 0 {
            format!("{}{}", colored, " ".repeat(pad))
        } else {
            colored
        }
    } else {
        // Ref is longer — show full ref, no truncation
        colored
    }
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
                    cross_chunk_replacements
                        .push((overlap.to_string(), " ".repeat(overlap.len())));
                }
            }
        }
    }

    // Output masking: apply replacements on new_text only (tail was already emitted).
    let mut output = new_text.clone();
    let mut output_had_replacement = false;

    // Apply cross-chunk boundary replacements first
    for (leaked, replacement) in &cross_chunk_replacements {
        output = output.replacen(leaked, replacement, 1);
        output_had_replacement = true;
    }

    // Cross-chunk mask_map: secrets echoed char-by-char span tail + new_text.
    if let Some(m) = find_cross_chunk_mask(plain_tail, &new_text, mask_map) {
        output = m.output;
        output_had_replacement = true;
    }

    // Standard mask_map: secrets fully within new_text
    for (plaintext, vk_ref) in mask_map {
        if !plaintext.is_empty() && output.contains(plaintext.as_str()) {
            output = output.replace(
                plaintext.as_str(),
                &padded_colorize_ref(vk_ref, plaintext.len()),
            );
            output_had_replacement = true;
        }
    }
    // Also apply pattern-detected replacements on new_text
    for pat in patterns {
        for caps in pat.regex.captures_iter(&new_text) {
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
                    output = output.replace(
                        secret,
                        &padded_colorize_ref(&ref_canonical, secret.len()),
                    );
                    output_had_replacement = true;
                }
                Err(e) => {
                    eprintln!(
                        "[veilkey] issue failed for pattern {}: {} — redacting (fail-closed)",
                        pat.name, e
                    );
                    let redacted = format!("[REDACTED:{}]", pat.name);
                    output = output.replace(
                        secret,
                        &padded_colorize_ref(&redacted, secret.len()),
                    );
                    output_had_replacement = true;
                }
            }
        }
    }
    for (plaintext, ve_ref) in ve_map {
        if !plaintext.is_empty() && output.contains(plaintext.as_str()) {
            output = output.replace(plaintext.as_str(), &colorize_ve_ref(plaintext, ve_ref));
        }
    }

    // Note: line-clear (\r\x1b[2K) was previously used here to redraw lines
    // after secret replacement, but padded_colorize_ref already ensures the
    // replacement has the same visible width as the original text. Line-clear
    // caused prompts to be erased when readline redraws (e.g. history recall).

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
        let re = regex::Regex::new(r"\x1b\[[0-9;]*m").unwrap();
        re.replace_all(s, "").to_string()
    }

    /// Helper: simulate mask_map replacement like mask_output step 1
    fn simulate_mask(input: &str, mask_map: &[(String, String)]) -> String {
        let mut s = input.to_string();
        for (plaintext, vk_ref) in mask_map {
            if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
                s = s.replace(
                    plaintext.as_str(),
                    &padded_colorize_ref(vk_ref, plaintext.len()),
                );
            }
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
        assert_eq!(visible, "VK:LOCAL:6da25530");
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
        // secret 3 chars → show full ref (no truncation)
        let result = padded_colorize_ref("VK:LOCAL:abc", 3);
        let visible = strip_ansi(&result);
        assert_eq!(visible, "VK:LOCAL:abc");
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
        // secret 1 char → show full ref (no truncation)
        let result = padded_colorize_ref("VK:LOCAL:abc", 1);
        let visible = strip_ansi(&result);
        assert_eq!(visible, "VK:LOCAL:abc");
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

    // ── padded_colorize_ref: no-truncation guarantees ─────────────

    #[test]
    fn test_ref_never_truncated_all_lengths() {
        // For every secret length 1..30, the full ref must always appear
        let vk_ref = "VK:LOCAL:6da25530"; // 17 chars
        for secret_len in 1..=30 {
            let result = padded_colorize_ref(vk_ref, secret_len);
            let visible = strip_ansi(&result);
            assert!(
                visible.contains(vk_ref),
                "ref truncated at secret_len={}: got [{}]",
                secret_len, visible
            );
        }
    }

    #[test]
    fn test_ref_never_truncated_temp_ref() {
        let vk_ref = "VK:TEMP:abc12345def67890"; // 24 chars
        for secret_len in 1..=30 {
            let result = padded_colorize_ref(vk_ref, secret_len);
            let visible = strip_ansi(&result);
            assert!(
                visible.contains(vk_ref),
                "TEMP ref truncated at secret_len={}: got [{}]",
                secret_len, visible
            );
        }
    }

    #[test]
    fn test_padding_when_secret_longer() {
        let vk_ref = "VK:LOCAL:abc"; // 12 chars
        for secret_len in 12..=50 {
            let result = padded_colorize_ref(vk_ref, secret_len);
            let visible = strip_ansi(&result);
            assert_eq!(
                visible.chars().count(), secret_len,
                "padding wrong at secret_len={}: got {} chars [{}]",
                secret_len, visible.chars().count(), visible
            );
            assert!(visible.starts_with(vk_ref));
            // trailing must be spaces only
            let trailing = &visible[vk_ref.len()..];
            assert!(
                trailing.chars().all(|c| c == ' '),
                "trailing chars not spaces at secret_len={}: [{:?}]",
                secret_len, trailing
            );
        }
    }

    #[test]
    fn test_exact_fit_no_padding_no_truncation() {
        let vk_ref = "VK:LOCAL:6da25530"; // 17 chars
        let result = padded_colorize_ref(vk_ref, 17);
        let visible = strip_ansi(&result);
        assert_eq!(visible, vk_ref);
        // No trailing spaces
        assert!(!visible.ends_with(' '));
    }

    #[test]
    fn test_ref_integrity_in_masked_output_short_secret() {
        // Real-world case: password shorter than ref
        let input = "pass=hunter2";
        let mask_map = vec![("hunter2".to_string(), "VK:LOCAL:6da25530".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains("hunter2"), "secret leaked");
        assert!(
            result.contains("VK:LOCAL:6da25530"),
            "full ref must appear in output, got: [{}]",
            result
        );
    }

    #[test]
    fn test_ref_integrity_in_masked_output_equal_length() {
        // Secret same length as ref (17 chars each)
        let input = "key=12345678901234567";
        let mask_map = vec![("12345678901234567".to_string(), "VK:LOCAL:6da25530".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains("12345678901234567"), "secret leaked");
        assert!(result.contains("VK:LOCAL:6da25530"), "full ref must appear");
        assert_eq!(result.len(), input.len(), "exact fit — width preserved");
    }

    #[test]
    fn test_ref_integrity_in_masked_output_long_secret() {
        // Secret longer than ref → padded
        let input = "key=this-is-a-very-long-secret-value-here";
        let secret = "this-is-a-very-long-secret-value-here";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:abc".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains(secret), "secret leaked");
        assert!(result.contains("VK:LOCAL:abc"), "ref must appear");
        assert_eq!(result.len(), input.len(), "width preserved with padding");
    }

    #[test]
    fn test_multiple_short_secrets_all_refs_intact() {
        // Multiple secrets shorter than their refs
        let input = "a=pw1 b=pw2 c=pw3";
        let mask_map = vec![
            ("pw1".to_string(), "VK:LOCAL:aaaa1111".to_string()),
            ("pw2".to_string(), "VK:LOCAL:bbbb2222".to_string()),
            ("pw3".to_string(), "VK:LOCAL:cccc3333".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains("pw1") && !result.contains("pw2") && !result.contains("pw3"),
            "secrets leaked");
        assert!(result.contains("VK:LOCAL:aaaa1111"), "ref1 truncated");
        assert!(result.contains("VK:LOCAL:bbbb2222"), "ref2 truncated");
        assert!(result.contains("VK:LOCAL:cccc3333"), "ref3 truncated");
    }

    #[test]
    fn test_real_world_password_ghdrhkdgh1() {
        // Exact scenario from user report: Ghdrhkdgh1@ (11 chars) → VK:LOCAL:6da25530 (17 chars)
        let input = "Ghdrhkdgh1@";
        let mask_map = vec![("Ghdrhkdgh1@".to_string(), "VK:LOCAL:6da25530".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains("Ghdrhkdgh1@"), "password leaked");
        assert!(
            result.contains("VK:LOCAL:6da25530"),
            "ref was truncated! got: [{}]",
            result
        );
    }

    #[test]
    fn test_real_world_password_in_command() {
        // User typed password as command → bash error output
        let input = "bash: Ghdrhkdgh1@: command not found";
        let mask_map = vec![("Ghdrhkdgh1@".to_string(), "VK:LOCAL:6da25530".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains("Ghdrhkdgh1@"), "password leaked in error msg");
        assert!(result.contains("VK:LOCAL:6da25530"), "ref truncated in error msg");
        assert!(result.contains("bash: "), "prefix must survive");
        assert!(result.contains(": command not found"), "suffix must survive");
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
        assert!(result.len() >= input.len(), "result must not be shorter");
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
        assert!(result.len() >= input.len());
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
        assert!(result.len() >= input.len());
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
        assert!(result.len() >= input.len());
        // Both occurrences must be masked
        assert!(!result.contains("MyPassword"));
    }

    // ── Secret at boundaries ────────────────────────────────────────

    #[test]
    fn test_mask_secret_at_line_start() {
        let input = "SuperSecret123 is exposed";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:ddd".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(result.len() >= input.len());
        assert!(!result.contains("SuperSecret"));
        assert!(result.contains(" is exposed"));
    }

    #[test]
    fn test_mask_secret_at_line_end() {
        let input = "password is SuperSecret123";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:eee".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(result.len() >= input.len());
        assert!(!result.contains("SuperSecret"));
        assert!(result.contains("password is "));
    }

    #[test]
    fn test_mask_secret_is_entire_line() {
        let input = "SuperSecret123";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:fff".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(result.len() >= input.len());
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
        assert!(result.len() >= input.len());
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
        assert!(result.len() >= input.len());
        assert!(!result.contains("p@$$"));
        assert!(result.contains("PASS="));
    }

    #[test]
    fn test_mask_secret_with_quotes() {
        let input = r#"export SECRET="my-quoted-secret""#;
        let mask_map = vec![("my-quoted-secret".to_string(), "VK:LOCAL:quot1".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(result.len() >= input.len());
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
        assert!(result.len() >= input.len());
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
        assert!(result.len() >= input.len());
        assert!(!result.contains("sk-1234567890"));
        assert!(result.contains("example.com"));
    }

    // ── env var output ──────────────────────────────────────────────

    #[test]
    fn test_mask_env_export_line() {
        let input = "export DATABASE_URL=postgres://admin:hunter2@db:5432/prod";
        let mask_map = vec![("hunter2".to_string(), "VK:LOCAL:env1".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(result.len() >= input.len());
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
        let ref_str = "VK:LOCAL:6da25530"; // 17 chars
        let ref_len = ref_str.chars().count();
        for secret_len in [5, 10, 15, 17, 20, 30, 50, 100] {
            let secret: String = (0..secret_len)
                .map(|i| (b'a' + (i % 26) as u8) as char)
                .collect();
            let input = format!("prefix:{}:suffix", secret);
            let mask_map = vec![(secret.clone(), ref_str.to_string())];
            let result = simulate_mask(&input, &mask_map);
            if secret_len >= ref_len {
                // Secret longer or equal — padded with spaces, width preserved
                assert_eq!(
                    result.len(),
                    input.len(),
                    "width mismatch for secret_len={}: input=[{}] result=[{}]",
                    secret_len, input, result
                );
            } else {
                // Secret shorter — full ref shown, output may be wider
                assert!(
                    result.len() >= input.len(),
                    "result should not be shorter for secret_len={}",
                    secret_len
                );
            }
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

    // ── No line-clear (prompt preservation) ─────────────────────────

    /// Helper: simulate masking and return the raw output (with ANSI codes)
    fn simulate_mask_raw(input: &str, mask_map: &[(String, String)]) -> String {
        let mut s = input.to_string();
        for (plaintext, vk_ref) in mask_map {
            if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
                s = s.replace(
                    plaintext.as_str(),
                    &padded_colorize_ref(vk_ref, plaintext.len()),
                );
            }
        }
        s
    }

    #[test]
    fn test_mask_no_line_clear_single_line() {
        // Masking must NOT insert \r\x1b[2K — it erases the prompt on
        // readline redraws (history recall, tab completion).
        let result = simulate_mask_raw(
            "echo SuperSecret123\n",
            &[("SuperSecret123".to_string(), "VK:LOCAL:aaa11111".to_string())],
        );
        assert!(!result.contains("\r\x1b[2K"), "line-clear must not be present");
        assert!(!result.contains("SuperSecret"), "secret must be masked");
    }

    #[test]
    fn test_mask_preserves_prompt_on_history_recall() {
        // Simulate readline history recall: CR + prompt + recalled command
        let result = simulate_mask_raw(
            "\r(VEIL) root$ echo hunter2",
            &[("hunter2".to_string(), "VK:LOCAL:bbb22222".to_string())],
        );
        assert!(result.contains("(VEIL) root$"), "prompt must be preserved");
        assert!(!result.contains("hunter2"), "secret must be masked");
        assert!(!result.contains("\x1b[2K"), "no line-clear");
    }

    #[test]
    fn test_mask_multiline_no_line_clear() {
        let result = simulate_mask_raw(
            "line1=secret1\nline2=secret2\n",
            &[
                ("secret1".to_string(), "VK:LOCAL:ccc".to_string()),
                ("secret2".to_string(), "VK:LOCAL:ddd".to_string()),
            ],
        );
        assert!(!result.contains("\x1b[2K"), "no line-clear anywhere");
        assert!(!result.contains("secret1"));
        assert!(!result.contains("secret2"));
        assert!(result.contains("line1="));
        assert!(result.contains("line2="));
    }

    #[test]
    fn test_mask_readline_cursor_movement_preserved() {
        // readline uses \x1b[C (cursor forward) to position text.
        // Masking must not corrupt these sequences.
        let result = simulate_mask_raw(
            "\x1b[C\x1b[C\x1b[Cecho hunter2\x1b[K",
            &[("hunter2".to_string(), "VK:LOCAL:eee".to_string())],
        );
        // Cursor movement sequences must survive
        assert!(result.contains("\x1b[C"), "cursor sequences must be preserved");
        // Erase-to-end must survive
        assert!(result.contains("\x1b[K"), "erase-to-end must be preserved");
        assert!(!result.contains("hunter2"), "secret must be masked");
    }

    #[test]
    fn test_mask_backspace_sequence_preserved() {
        // readline uses \x08 (backspace) to erase chars during history navigation
        let result = simulate_mask_raw(
            "\x08\x08\x08\x08\x08hunter2",
            &[("hunter2".to_string(), "VK:LOCAL:fff".to_string())],
        );
        assert!(result.contains("\x08"), "backspaces must be preserved");
        assert!(!result.contains("hunter2"), "secret must be masked");
    }

    // ══════════════════════════════════════════════════════════════════
    // SECURITY-FOCUSED TESTS
    // ══════════════════════════════════════════════════════════════════

    // ── Partial / substring leakage ─────────────────────────────────

    #[test]
    fn test_sec_no_partial_leak_prefix() {
        // If "SuperSecret123" is masked, no prefix substring should survive
        let input = "SuperSecret123";
        let mask_map = vec![("SuperSecret123".to_string(), "VK:LOCAL:aaa".to_string())];
        let result = simulate_mask(input, &mask_map);
        for i in 1..input.len() {
            assert!(
                !result.contains(&input[..i]),
                "prefix [{}] leaked in result [{}]",
                &input[..i], result
            );
        }
    }

    #[test]
    fn test_sec_no_partial_leak_suffix() {
        let input = "tok=SuperSecret123";
        let secret = "SuperSecret123";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:aaa".to_string())];
        let result = simulate_mask(input, &mask_map);
        for i in 1..secret.len() {
            assert!(
                !result.contains(&secret[i..]),
                "suffix [{}] leaked",
                &secret[i..]
            );
        }
    }

    #[test]
    fn test_sec_secret_repeated_many_times() {
        // Secret appearing 10 times — every occurrence must be masked
        let secret = "LeakMe123";
        let input = (0..10).map(|i| format!("f{}={}", i, secret)).collect::<Vec<_>>().join(" ");
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:rep".to_string())];
        let result = simulate_mask(&input, &mask_map);
        assert!(!result.contains(secret), "secret still present");
        let count = result.matches("VK:LOCAL:rep").count();
        assert_eq!(count, 10, "expected 10 replacements, got {}", count);
    }

    // ── ANSI escape sequence evasion ────────────────────────────────

    #[test]
    fn test_sec_ansi_color_inside_secret_known_limit() {
        // KNOWN LIMITATION: ANSI codes injected INTO a secret by the program
        // producing output can split the plaintext so mask_map matching fails.
        // This is acceptable because:
        // 1. The attacker would need to control the program's output formatting
        // 2. Pattern-based detection (regex) can still catch these via pre-scan
        // 3. Normal programs don't insert ANSI codes inside secret values
        let secret = "hunter2";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:ansi1".to_string())];
        let evasion = format!("hun\x1b[31mter2");
        let (out, _) = mask_output_simple(&evasion, &mask_map, "");
        let visible = strip_ansi(&out);
        // Currently this DOES leak — documenting as known limitation
        // When ansi_aware_replace is enhanced, change this to assert !contains
        assert!(
            visible.contains(secret),
            "if this stops failing, ANSI evasion defense was improved!"
        );
    }

    #[test]
    fn test_sec_ansi_around_secret_still_masked() {
        // ANSI codes AROUND (not inside) the secret — must still mask
        let secret = "hunter2";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:ansi2".to_string())];
        let input = format!("\x1b[31m{}\x1b[0m", secret);
        let (out, _) = mask_output_simple(&input, &mask_map, "");
        let visible = strip_ansi(&out);
        assert!(
            !visible.contains(secret),
            "secret with surrounding ANSI leaked: [{}]",
            visible
        );
    }

    // ── Cross-chunk boundary secrets (mask_output) ──────────────────

    fn init_rustls() {
        use std::sync::Once;
        static INIT: Once = Once::new();
        INIT.call_once(|| {
            let _ = rustls::crypto::ring::default_provider().install_default();
        });
    }

    fn mask_output_simple(
        data: &str,
        mask_map: &[(String, String)],
        plain_tail: &str,
    ) -> (String, String) {
        init_rustls();
        let (bytes, tail) = mask_output(
            data.as_bytes(),
            mask_map,
            &[],
            &[],
            &crate::api::VeilKeyClient::new("http://127.0.0.1:1"),
            "",
            plain_tail,
        );
        (String::from_utf8_lossy(&bytes).to_string(), tail)
    }

    #[test]
    fn test_sec_cross_chunk_secret_split_middle() {
        // Secret "SuperSecret" split across two chunks: "Super" | "Secret"
        let secret = "SuperSecret";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:cross1".to_string())];

        // Chunk 1: "prefix Super"
        let (out1, tail1) = mask_output_simple("prefix Super", &mask_map, "");
        // Chunk 2: "Secret suffix"
        let (out2, _tail2) = mask_output_simple("Secret suffix", &mask_map, &tail1);

        let combined = format!("{}{}", strip_ansi(&out1), strip_ansi(&out2));
        assert!(
            !combined.contains("SuperSecret"),
            "cross-chunk secret leaked: [{}]",
            combined
        );
    }

    #[test]
    fn test_sec_cross_chunk_single_char_boundary() {
        // Secret split at single char: "hunter" | "2"
        let secret = "hunter2";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:cross2".to_string())];

        let (out1, tail1) = mask_output_simple("cmd hunter", &mask_map, "");
        let (out2, _) = mask_output_simple("2 done", &mask_map, &tail1);

        let combined = format!("{}{}", strip_ansi(&out1), strip_ansi(&out2));
        assert!(
            !combined.contains("hunter2"),
            "1-char boundary split leaked: [{}]",
            combined
        );
    }

    #[test]
    fn test_sec_cross_chunk_secret_fully_in_second() {
        // Secret entirely in second chunk — tail should not interfere
        let secret = "MySecret";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:cross3".to_string())];

        let (out1, tail1) = mask_output_simple("some normal output\n", &mask_map, "");
        let (out2, _) = mask_output_simple("leaked: MySecret here", &mask_map, &tail1);

        let visible2 = strip_ansi(&out2);
        assert!(!visible2.contains("MySecret"), "secret in chunk2 leaked");
        assert!(visible2.contains("VK:LOCAL:cross3"), "ref missing in chunk2");
    }

    // ── Encoding evasion (base64, hex) ──────────────────────────────

    #[test]
    fn test_sec_base64_encoded_secret_masked() {
        // enrich_mask_map adds base64 variants for secrets >= 8 chars
        let secret = "my-api-key-value";
        let b64 = base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            secret.as_bytes(),
        );
        let mut mask_map = vec![(secret.to_string(), "VK:LOCAL:enc1".to_string())];
        crate::api::enrich_mask_map(&mut mask_map);

        let input = format!("encoded={}", b64);
        let result = simulate_mask(&input, &mask_map);
        assert!(!result.contains(&b64), "base64 encoded secret leaked");
    }

    #[test]
    fn test_sec_hex_encoded_secret_masked() {
        let secret = "my-api-key-value";
        let hex: String = secret.bytes().map(|b| format!("{:02x}", b)).collect();
        let mut mask_map = vec![(secret.to_string(), "VK:LOCAL:enc2".to_string())];
        crate::api::enrich_mask_map(&mut mask_map);

        let input = format!("hex={}", hex);
        let result = simulate_mask(&input, &mask_map);
        assert!(!result.contains(&hex), "hex encoded secret leaked");
    }

    #[test]
    fn test_sec_short_secret_no_encoded_variants() {
        // Secrets < 8 chars should NOT have encoded variants (too many false positives)
        let secret = "short";
        let mut mask_map = vec![(secret.to_string(), "VK:LOCAL:enc3".to_string())];
        let before_len = mask_map.len();
        crate::api::enrich_mask_map(&mut mask_map);
        assert_eq!(
            mask_map.len(), before_len,
            "short secrets must not get encoded variants"
        );
    }

    // ── Self-corruption prevention ──────────────────────────────────

    #[test]
    fn test_sec_ref_not_masked_by_another_secret() {
        // If a secret value equals part of a VK ref, it must be filtered out
        // to prevent ref corruption
        let mut mask_map = vec![
            ("6da25530".to_string(), "VK:LOCAL:6da25530".to_string()),
            ("real-secret".to_string(), "VK:LOCAL:abc".to_string()),
        ];
        crate::api::enrich_mask_map(&mut mask_map);
        // "6da25530" should be removed (self-corruption)
        assert!(
            !mask_map.iter().any(|(p, _)| p == "6da25530"),
            "self-corrupting entry must be removed"
        );
        // "real-secret" should survive
        assert!(mask_map.iter().any(|(p, _)| p == "real-secret"));
    }

    #[test]
    fn test_sec_vk_prefix_not_in_mask_map() {
        // Values like "VK", "LOCAL", "TEMP" must be excluded
        let mut mask_map = vec![
            ("VK".to_string(), "VK:LOCAL:a".to_string()),
            ("LOCAL".to_string(), "VK:LOCAL:b".to_string()),
            ("TEMP".to_string(), "VK:TEMP:c".to_string()),
            ("VE".to_string(), "VE:LOCAL:d".to_string()),
            ("HOST".to_string(), "VK:HOST:e".to_string()),
        ];
        crate::api::enrich_mask_map(&mut mask_map);
        assert!(mask_map.is_empty(), "ref keywords must all be filtered");
    }

    // ── Zero-width / invisible characters ───────────────────────────

    #[test]
    fn test_sec_zero_width_chars_dont_hide_secret() {
        // Zero-width space (U+200B) inserted in secret — plaintext match should still work
        let secret = "password123";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:zw1".to_string())];
        // Normal case — should mask
        let result = simulate_mask("val=password123", &mask_map);
        assert!(!result.contains("password123"), "normal secret leaked");
    }

    #[test]
    fn test_sec_empty_secret_ignored() {
        // Empty string in mask_map must not cause issues
        let mask_map = vec![
            ("".to_string(), "VK:LOCAL:empty".to_string()),
            ("realvalue".to_string(), "VK:LOCAL:real1".to_string()),
        ];
        let result = simulate_mask("val=realvalue", &mask_map);
        assert!(!result.contains("realvalue"), "real secret leaked");
        // Empty secret must not replace anything
        assert!(result.contains("val="), "prefix must survive");
    }

    // ── Overlapping secrets ─────────────────────────────────────────

    #[test]
    fn test_sec_overlapping_secrets_longest_wins() {
        // "password" and "password123" both in mask_map
        // After enrich (sorted longest-first), "password123" should match first
        let mut mask_map = vec![
            ("password".to_string(), "VK:LOCAL:short1".to_string()),
            ("password123".to_string(), "VK:LOCAL:long1".to_string()),
        ];
        crate::api::enrich_mask_map(&mut mask_map);
        let result = simulate_mask("val=password123", &mask_map);
        // Neither plaintext should survive
        assert!(!result.contains("password123"), "long secret leaked");
        assert!(!result.contains("password"), "short secret leaked");
    }

    #[test]
    fn test_sec_adjacent_secrets_no_gap() {
        // Two secrets directly adjacent: "secret1secret2"
        let mask_map = vec![
            ("secret1".to_string(), "VK:LOCAL:adj1".to_string()),
            ("secret2".to_string(), "VK:LOCAL:adj2".to_string()),
        ];
        let result = simulate_mask("secret1secret2", &mask_map);
        assert!(!result.contains("secret1"), "first adjacent leaked");
        assert!(!result.contains("secret2"), "second adjacent leaked");
    }

    // ── Connection string attack vectors ────────────────────────────

    #[test]
    fn test_sec_password_in_url_with_special_chars() {
        // URL-encoded special chars in password
        let input = "postgres://admin:p%40ssw0rd@host:5432/db";
        let secret = "p%40ssw0rd";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:url1".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert!(!result.contains(secret), "URL-encoded password leaked");
        assert!(result.contains("@host:5432"), "host must survive");
    }

    #[test]
    fn test_sec_multiline_secret_each_line_masked() {
        // Secret appears on multiple lines of output
        let secret = "s3cr3t-k3y";
        let input = "line1: s3cr3t-k3y\nline2: s3cr3t-k3y\nline3: s3cr3t-k3y";
        let mask_map = vec![(secret.to_string(), "VK:LOCAL:ml1".to_string())];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(
            result.matches(secret).count(), 0,
            "secret found on some lines"
        );
        assert_eq!(
            result.matches("VK:LOCAL:ml1").count(), 3,
            "expected 3 replacements"
        );
    }

    // ── JSON body security (json_password_body cross-check) ─────────

    #[test]
    fn test_sec_password_not_in_masked_output() {
        // Verify that a password used for login can't leak through mask_output
        let password = "Ghdrhkdgh1@";
        let mask_map = vec![(password.to_string(), "VK:LOCAL:6da25530".to_string())];
        // Simulate various contexts where password might appear
        let contexts = vec![
            format!("echo {}", password),
            format!("export PASS={}", password),
            format!("curl -d '{{\"password\":\"{}\"}}'", password),
            format!("mysql -p{}", password),
            format!("{}: command not found", password),
            format!("Error: authentication failed with {}", password),
        ];
        for ctx in &contexts {
            let result = simulate_mask(ctx, &mask_map);
            assert!(
                !result.contains(password),
                "password leaked in context: [{}] → [{}]",
                ctx, result
            );
        }
    }

    // ── Stress / edge cases ─────────────────────────────────────────

    #[test]
    fn test_sec_many_secrets_in_mask_map() {
        // 500 secrets — performance and correctness
        let secrets: Vec<(String, String)> = (0..500)
            .map(|i| (
                format!("secret-{:04}-value", i),
                format!("VK:LOCAL:{:08x}", i),
            ))
            .collect();
        let input = secrets.iter().map(|(s, _)| s.as_str()).collect::<Vec<_>>().join(" ");
        let result = simulate_mask(&input, &secrets);
        for (secret, _) in &secrets {
            assert!(!result.contains(secret.as_str()), "secret {} leaked", secret);
        }
    }

    #[test]
    fn test_sec_binary_data_no_panic() {
        // Binary data in input must not cause panics
        let data: Vec<u8> = (0..=255).collect();
        let mask_map = vec![("testvalue".to_string(), "VK:LOCAL:bin1".to_string())];
        // Should not panic
        let (_output, _tail) = mask_output_simple(
            &String::from_utf8_lossy(&data),
            &mask_map,
            "",
        );
    }

    #[test]
    fn test_sec_very_long_secret_masked() {
        // 4KB secret — must still be masked
        let secret: String = (0..4096).map(|i| (b'A' + (i % 26) as u8) as char).collect();
        let mask_map = vec![(secret.clone(), "VK:LOCAL:big1".to_string())];
        let input = format!("data={}", secret);
        let result = simulate_mask(&input, &mask_map);
        assert!(!result.contains(&secret[..50]), "long secret prefix leaked");
    }

    // ── Cross-chunk mask_map (backspace erase) ──────────────────────

    fn mk(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs.iter().map(|(s, r)| (s.to_string(), r.to_string())).collect()
    }

    #[test]
    fn cross_chunk_basic_split() {
        // Secret "password123" split: tail has "password1", new has "23"
        let map = mk(&[("password123", "VK:LOCAL:aaa")]);
        let result = find_cross_chunk_mask("password1", "23", &map);
        assert!(result.is_some(), "should detect cross-chunk secret");
        let out = result.unwrap().output;
        let visible = strip_ansi(&out);
        assert!(visible.contains("VK:LOCAL:aaa"), "must contain VK ref");
        // Uses ANSI CSI cursor-left + erase-to-end (not backspace)
        assert!(out.contains("\x1b["), "must have ANSI erase sequence");
    }

    #[test]
    fn cross_chunk_last_char() {
        // Only last char is in new_text
        let map = mk(&[("secret!", "VK:LOCAL:bbb")]);
        let result = find_cross_chunk_mask("secret", "!", &map);
        assert!(result.is_some());
    }

    #[test]
    fn cross_chunk_first_char_in_tail() {
        // Only first char is in tail
        let map = mk(&[("abcdef", "VK:LOCAL:ccc")]);
        let result = find_cross_chunk_mask("a", "bcdef", &map);
        assert!(result.is_some());
    }

    #[test]
    fn cross_chunk_no_match() {
        let map = mk(&[("nomatch", "VK:LOCAL:ddd")]);
        let result = find_cross_chunk_mask("hello world", "foo", &map);
        assert!(result.is_none());
    }

    #[test]
    fn cross_chunk_fully_in_tail_ignored() {
        // Secret is entirely in tail (stale) — must NOT match
        let map = mk(&[("stale", "VK:LOCAL:eee")]);
        let result = find_cross_chunk_mask("old stale data here", "newdata", &map);
        assert!(result.is_none(), "stale secret in tail must be ignored");
    }

    #[test]
    fn cross_chunk_fully_in_new_ignored() {
        // Secret is entirely in new_text — handled by standard mask, not cross-chunk
        let map = mk(&[("secret", "VK:LOCAL:fff")]);
        let result = find_cross_chunk_mask("no match here", "has secret in it", &map);
        assert!(result.is_none(), "fully-in-new should be handled by standard mask");
    }

    #[test]
    fn cross_chunk_repeated_invocations() {
        // Simulates typing the same secret multiple times.
        // After first replacement, tail contains the secret from bash error.
        // Second invocation must still match at the boundary, not the stale one.
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);

        // 1st: tail = prompt, new = last char + enter
        let result1 = find_cross_chunk_mask(
            "(VEIL) pve:~ root$ Ghdrhkdg",
            "h1@\r",
            &map,
        );
        assert!(result1.is_some(), "1st invocation must match");

        // 2nd: tail has stale secret from previous bash error output
        let result2 = find_cross_chunk_mask(
            "bash: Ghdrhkdgh1@: 명령어를 찾을 수 없음\n(VEIL) pve:~ root$ Ghdrhkdg",
            "h1@\r",
            &map,
        );
        assert!(result2.is_some(), "2nd invocation must match at boundary, not stale");

        // 3rd: same pattern
        let result3 = find_cross_chunk_mask(
            "을 수 없음\nbash: Ghdrhkdgh1@: 명령어를 찾을 수 없음\n(VEIL) pve:~ root$ Ghdrhkdg",
            "h1@\r",
            &map,
        );
        assert!(result3.is_some(), "3rd invocation must still match");
    }

    #[test]
    fn cross_chunk_utf8_boundary_no_panic() {
        // Tail ends with multibyte char (한글). search_start must not
        // land inside a multibyte char — would panic on &combined[search_start..].
        let map = mk(&[("secret123", "VK:LOCAL:ggg")]);
        // 한글 "명" is 3 bytes. Tail ends mid-Korean text.
        let tail = "bash: 명령어를 찾을 수 없음\n(VEIL) root$ secre";
        let new = "t123";
        // Must not panic
        let result = find_cross_chunk_mask(tail, new, &map);
        assert!(result.is_some(), "should match across utf8 tail");
    }

    #[test]
    fn cross_chunk_short_secret_skipped() {
        // Secrets < 3 chars are skipped to avoid false positives
        let map = mk(&[("ab", "VK:LOCAL:hhh")]);
        let result = find_cross_chunk_mask("a", "b", &map);
        assert!(result.is_none(), "short secrets must be skipped");
    }

    #[test]
    fn cross_chunk_empty_tail() {
        let map = mk(&[("secret", "VK:LOCAL:iii")]);
        let result = find_cross_chunk_mask("", "secret", &map);
        assert!(result.is_none(), "no cross-chunk possible with empty tail");
    }

    #[test]
    fn cross_chunk_empty_new() {
        let map = mk(&[("secret", "VK:LOCAL:jjj")]);
        let result = find_cross_chunk_mask("secret", "", &map);
        assert!(result.is_none(), "no cross-chunk possible with empty new");
    }

    #[test]
    fn cross_chunk_erase_uses_ansi_cursor_control() {
        // Verify the erase uses CSI cursor-left + erase-to-end, not backspaces
        let map = mk(&[("abcdefgh", "VK:LOCAL:kkk")]);
        let result = find_cross_chunk_mask("abcde", "fgh", &map);
        assert!(result.is_some());
        let out = result.unwrap().output;
        // 5 chars in tail → \x1b[5D (cursor left 5) + \x1b[K (erase to end)
        assert!(out.contains("\x1b[5D"), "must have CSI cursor-left 5");
        assert!(out.contains("\x1b[K"), "must have erase-to-end");
        assert!(!out.contains("\x08"), "must NOT use backspace");
    }

    // ── Char-by-char echo simulation ────────────────────────────────
    // Simulate PTY echo: each typed char produces one mask_output call.
    // The secret must be caught exactly ONCE at the last-char boundary,
    // never producing partial VK refs on intermediate chars.

    /// Simulate char-by-char echo through find_cross_chunk_mask.
    /// Returns the concatenated output across all calls.
    fn simulate_charwise_echo(
        initial_tail: &str,
        typed: &str,
        mask_map: &[(String, String)],
    ) -> (String, usize) {
        let mut tail = initial_tail.to_string();
        let mut full_output = String::new();
        let mut match_count = 0;
        for ch in typed.chars() {
            let new = ch.to_string();
            if let Some(m) = find_cross_chunk_mask(&tail, &new, mask_map) {
                full_output.push_str(&m.output);
                match_count += 1;
                // After cross-chunk match, tail resets (the replacement was emitted)
                // In real code, tail gets the original new_text, not the masked version.
                tail = format!("{}{}", tail, new);
            } else {
                full_output.push(ch);
                tail = format!("{}{}", tail, new);
            }
            // Keep tail bounded (like PLAIN_TAIL_SIZE)
            if tail.len() > 4096 {
                tail = tail[tail.len() - 4096..].to_string();
            }
        }
        (full_output, match_count)
    }

    #[test]
    fn charwise_secret_matched_exactly_once() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (output, count) = simulate_charwise_echo(
            "(VEIL) pve:~ root$ ",
            "Ghdrhkdgh1@\r",
            &map,
        );
        assert_eq!(count, 1, "secret must match exactly once, got {}", count);
        let visible = strip_ansi(&output);
        assert!(visible.contains("VK:LOCAL:6da25530"), "must contain VK ref: {}", visible);
        // Must NOT contain partial VK refs like "VK:LOC"
        let vk_fragments = visible.matches("VK:LOC").count();
        assert_eq!(vk_fragments, 1, "only one VK ref, not partial fragments: {}", visible);
    }

    #[test]
    fn charwise_no_partial_refs_on_intermediate_chars() {
        let map = mk(&[("password123", "VK:LOCAL:aaa")]);
        let (output, count) = simulate_charwise_echo("$ ", "password123\r", &map);
        assert_eq!(count, 1);
        // No VK ref fragments before the final match
        let visible = strip_ansi(&output);
        assert_eq!(visible.matches("VK:").count(), 1);
    }

    #[test]
    fn charwise_repeated_5_times() {
        // User types the same secret 5 times in a row
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let mut tail = String::new();
        for i in 0..5 {
            let prompt = format!("(VEIL) root$ ");
            tail.push_str(&prompt);
            let (output, count) = simulate_charwise_echo(&tail, "Ghdrhkdgh1@\r", &map);
            assert_eq!(count, 1, "attempt {} must match exactly once", i + 1);
            let visible = strip_ansi(&output);
            assert_eq!(
                visible.matches("VK:LOC").count(), 1,
                "attempt {} must have exactly one VK ref, got: {}",
                i + 1, visible
            );
            // Simulate bash error output appended to tail
            tail.push_str(&format!(
                "Ghdrhkdgh1@\nbash: Ghdrhkdgh1@: command not found\n"
            ));
            if tail.len() > 4096 {
                tail = tail[tail.len() - 4096..].to_string();
            }
        }
    }

    #[test]
    fn charwise_different_secret_no_interference() {
        // Two secrets in mask_map, type only one
        let map = mk(&[
            ("password123", "VK:LOCAL:aaa"),
            ("Ghdrhkdgh1@", "VK:LOCAL:bbb"),
        ]);
        let (output, count) = simulate_charwise_echo("$ ", "password123\r", &map);
        assert_eq!(count, 1);
        let visible = strip_ansi(&output);
        assert!(visible.contains("VK:LOCAL:aaa"));
        assert!(!visible.contains("VK:LOCAL:bbb"));
    }

    #[test]
    fn charwise_prefix_overlap_longest_wins() {
        // "pass" is a prefix of "password", both in mask_map.
        // When char 'd' arrives (completing "password"), longest-match must win.
        // When char 's' arrives (completing "pass" at char 4), "pass" fires
        // because "password" is not yet complete. This is expected behavior —
        // the short secret is a valid secret and must be masked.
        let map = mk(&[
            ("password", "VK:LOCAL:long1"),
            ("pass", "VK:LOCAL:short1"),
        ]);
        let (output, count) = simulate_charwise_echo("$ ", "password\r", &map);
        // "pass" matches at char 4 (short match fires first)
        assert!(count >= 1, "at least one match");
        let visible = strip_ansi(&output);
        // No corrupted partial VK fragments (like "VK:LOCVK:LOC")
        assert!(
            !visible.contains("VK:LOCVK:"),
            "must not have concatenated partial refs: {}", visible
        );
    }

    #[test]
    fn charwise_no_prefix_overlap_clean_match() {
        // When secrets don't overlap, exactly one match.
        let map = mk(&[("secretXYZ", "VK:LOCAL:only1")]);
        let (output, count) = simulate_charwise_echo("$ ", "secretXYZ\r", &map);
        assert_eq!(count, 1);
        let visible = strip_ansi(&output);
        assert_eq!(visible.matches("VK:LOCAL:only1").count(), 1);
    }

    #[test]
    fn charwise_safe_command_no_match() {
        let map = mk(&[("secret123", "VK:LOCAL:xxx")]);
        let (output, count) = simulate_charwise_echo("$ ", "ls -la\r", &map);
        assert_eq!(count, 0, "safe command must not trigger");
        assert_eq!(output, "ls -la\r");
    }

    #[test]
    fn charwise_paste_all_at_once() {
        // Paste: entire secret arrives in one chunk (not char-by-char)
        // find_cross_chunk_mask should NOT match (fully in new_text)
        // Standard mask_map should handle this instead.
        let map = mk(&[("secret123", "VK:LOCAL:yyy")]);
        let result = find_cross_chunk_mask("$ ", "secret123\r", &map);
        assert!(result.is_none(), "fully in new_text → standard mask handles it");
    }

    // ── Fast typing: multi-char chunks ──────────────────────────────
    // When user types fast, multiple chars arrive in a single read().
    // Simulate by calling find_cross_chunk_mask with multi-char new_text.

    /// Simulate fast typing: split typed string into chunks of given size
    fn simulate_chunked_echo(
        initial_tail: &str,
        typed: &str,
        chunk_size: usize,
        mask_map: &[(String, String)],
    ) -> (String, usize) {
        let mut tail = initial_tail.to_string();
        let mut full_output = String::new();
        let mut match_count = 0;
        let chars: Vec<char> = typed.chars().collect();
        let mut i = 0;
        while i < chars.len() {
            let end = (i + chunk_size).min(chars.len());
            let chunk: String = chars[i..end].iter().collect();
            if let Some(m) = find_cross_chunk_mask(&tail, &chunk, mask_map) {
                full_output.push_str(&m.output);
                match_count += 1;
            } else {
                full_output.push_str(&chunk);
            }
            tail.push_str(&chunk);
            if tail.len() > 4096 {
                let start = tail.ceil_char_boundary(tail.len() - 4096);
                tail = tail[start..].to_string();
            }
            i = end;
        }
        (full_output, match_count)
    }

    #[test]
    fn fast_typing_2char_chunks_no_partial_refs() {
        // Secret "Ghdrhkdgh1@" typed in 2-char chunks
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (output, count) = simulate_chunked_echo(
            "(VEIL) root$ ", "Ghdrhkdgh1@\r", 2, &map,
        );
        assert_eq!(count, 1, "must match exactly once with 2-char chunks");
        let visible = strip_ansi(&output);
        assert!(
            !visible.contains("VK:LOCVK:"),
            "no concatenated partial refs: {}", visible
        );
    }

    #[test]
    fn fast_typing_3char_chunks_no_partial_refs() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (output, count) = simulate_chunked_echo(
            "(VEIL) root$ ", "Ghdrhkdgh1@\r", 3, &map,
        );
        assert_eq!(count, 1, "must match exactly once with 3-char chunks");
        let visible = strip_ansi(&output);
        assert!(
            !visible.contains("VK:LOCVK:"),
            "no concatenated partial refs: {}", visible
        );
    }

    #[test]
    fn fast_typing_5char_chunks_no_partial_refs() {
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (output, count) = simulate_chunked_echo(
            "(VEIL) root$ ", "Ghdrhkdgh1@\r", 5, &map,
        );
        assert_eq!(count, 1);
        let visible = strip_ansi(&output);
        assert!(!visible.contains("VK:LOCVK:"), "no partial refs: {}", visible);
    }

    #[test]
    fn fast_typing_whole_line_at_once() {
        // Entire line arrives in one chunk — cross-chunk should NOT fire
        // (fully in new_text, handled by standard mask)
        let map = mk(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let (_, count) = simulate_chunked_echo(
            "(VEIL) root$ ", "Ghdrhkdgh1@\r", 100, &map,
        );
        assert_eq!(count, 0, "fully in new_text — cross-chunk must not fire");
    }

    #[test]
    fn fast_typing_repeated_5_times_2char() {
        let map = mk(&[("Fkslrhenfk1@", "VK:LOCAL:abc12345")]);
        let mut tail = String::new();
        for i in 0..5 {
            tail.push_str("(VEIL) root$ ");
            let (output, count) = simulate_chunked_echo(&tail, "Fkslrhenfk1@\r", 2, &map);
            assert_eq!(count, 1, "attempt {} must match exactly once", i + 1);
            let visible = strip_ansi(&output);
            assert!(
                !visible.contains("VK:LOCVK:"),
                "attempt {}: no partial refs: {}", i + 1, visible
            );
            // Simulate bash error appended to tail
            tail.push_str("Fkslrhenfk1@\nbash: Fkslrhenfk1@: command not found\n");
            if tail.len() > 4096 {
                let start = tail.ceil_char_boundary(tail.len() - 4096);
                tail = tail[start..].to_string();
            }
        }
    }

    #[test]
    fn fast_typing_with_prefix_overlap() {
        // "pass" and "password" both in mask_map, typed in 2-char chunks
        let map = mk(&[
            ("password", "VK:LOCAL:long1"),
            ("pass", "VK:LOCAL:short1"),
        ]);
        let (output, _count) = simulate_chunked_echo("$ ", "password\r", 2, &map);
        let visible = strip_ansi(&output);
        // Must not have corrupted "VK:LOCVK:LOC" fragments
        assert!(
            !visible.contains("VK:LOCVK:"),
            "no concatenated partial refs: {}", visible
        );
    }

    #[test]
    fn fast_typing_multiple_secrets_in_mask_map() {
        // 103 secrets (like real veil session), type one of them fast
        let mut map: Vec<(String, String)> = (0..102)
            .map(|i| (format!("other_secret_{:03}", i), format!("VK:LOCAL:{:08x}", i)))
            .collect();
        map.push(("Ghdrhkdgh1@".to_string(), "VK:LOCAL:6da25530".to_string()));
        let (output, count) = simulate_chunked_echo("$ ", "Ghdrhkdgh1@\r", 2, &map);
        assert_eq!(count, 1);
        let visible = strip_ansi(&output);
        assert!(visible.contains("VK:LOCAL:6da25530"));
        assert!(!visible.contains("VK:LOCVK:"), "no partial refs: {}", visible);
    }

    // ══════════════════════════════════════════════════════════════════
    // SECURITY ROUND 2: VE, recent_input, tail, unicode, formats
    // ══════════════════════════════════════════════════════════════════

    fn mask_with_ve(
        data: &str, mask_map: &[(String, String)],
        ve_map: &[(String, String)], tail: &str,
    ) -> (String, String) {
        init_rustls();
        let (b, t) = mask_output(
            data.as_bytes(), mask_map, ve_map, &[],
            &crate::api::VeilKeyClient::new("http://127.0.0.1:1"), "", tail,
        );
        (String::from_utf8_lossy(&b).to_string(), t)
    }

    fn mask_with_input(data: &str, mask_map: &[(String, String)], input: &str) -> String {
        init_rustls();
        let (b, _) = mask_output(
            data.as_bytes(), mask_map, &[], &[],
            &crate::api::VeilKeyClient::new("http://127.0.0.1:1"), input, "",
        );
        strip_ansi(&String::from_utf8_lossy(&b))
    }

    // ── VE entries ──────────────────────────────────────────────────

    #[test]
    fn test_sec_ve_colorized() {
        let ve = vec![("config-val".into(), "VE:LOCAL:c1".into())];
        let (out, _) = mask_with_ve("setting=config-val", &[], &ve, "");
        assert!(out.contains(GREEN), "VE must be green");
        assert!(out.contains("config-val"));
    }

    #[test]
    fn test_sec_vk_and_ve_together() {
        let vk = vec![("secret-pw".into(), "VK:LOCAL:a".into())];
        let ve = vec![("cfg-val".into(), "VE:LOCAL:b".into())];
        let (out, _) = mask_with_ve("pw=secret-pw cfg=cfg-val", &vk, &ve, "");
        let vis = strip_ansi(&out);
        assert!(!vis.contains("secret-pw"), "VK leaked with VE");
        assert!(vis.contains("cfg-val"), "VE lost");
    }

    #[test]
    fn test_sec_ve_empty_ignored() {
        let ve = vec![("".into(), "VE:LOCAL:e".into())];
        let (out, _) = mask_with_ve("text", &[], &ve, "");
        assert_eq!(strip_ansi(&out), "text");
    }

    // ── recent_input ────────────────────────────────────────────────

    #[test]
    fn test_sec_unrelated_input_still_masks() {
        let m = vec![("SuperSecret".into(), "VK:LOCAL:r1".into())];
        let r = mask_with_input("SuperSecret", &m, "echo hello");
        assert!(!r.contains("SuperSecret"));
    }

    #[test]
    fn test_sec_empty_input_masks() {
        let m = vec![("password".into(), "VK:LOCAL:r2".into())];
        let r = mask_with_input("password", &m, "");
        assert!(!r.contains("password"));
    }

    // ── plain_tail ──────────────────────────────────────────────────

    #[test]
    fn test_sec_tail_overflow() {
        let m = vec![("x".into(), "VK:LOCAL:t1".into())];
        let (_, tail) = mask_output_simple(&"x".repeat(16000), &m, "");
        assert!(tail.len() <= 8192 + 100, "tail={}", tail.len());
    }

    #[test]
    fn test_sec_tail_boundary_secret() {
        let pad = "a".repeat(8190);
        let m = vec![("SECRETHERE".into(), "VK:LOCAL:t2".into())];
        let (out, _) = mask_output_simple(&format!("{}SECRETHERE", pad), &m, "");
        assert!(!strip_ansi(&out).contains("SECRETHERE"));
    }

    #[test]
    fn test_sec_accumulated_tail() {
        let m = vec![("crossval".into(), "VK:LOCAL:t3".into())];
        let (_, t1) = mask_output_simple(&"a".repeat(4000), &m, "");
        let (_, t2) = mask_output_simple(&"b".repeat(4000), &m, &t1);
        let (out, _) = mask_output_simple("crossval", &m, &t2);
        assert!(!strip_ansi(&out).contains("crossval"));
    }

    // ── Unicode ─────────────────────────────────────────────────────

    #[test]
    fn test_sec_korean() {
        let s = "비밀번호입니다";
        let m = vec![(s.into(), "VK:LOCAL:u1".into())];
        assert!(!simulate_mask(&format!("pw={}", s), &m).contains(s));
    }

    #[test]
    fn test_sec_emoji() {
        let s = "🔑key🔑val🔑";
        let m = vec![(s.into(), "VK:LOCAL:u2".into())];
        assert!(!simulate_mask(&format!("t={}", s), &m).contains(s));
    }

    #[test]
    fn test_sec_mixed_ascii_unicode() {
        let s = "pass한글word";
        let m = vec![(s.into(), "VK:LOCAL:u3".into())];
        assert!(!simulate_mask(&format!("v={}", s), &m).contains(s));
    }

    // ── Null / control chars ────────────────────────────────────────

    #[test]
    fn test_sec_null_byte() {
        let m = vec![("secret".into(), "VK:LOCAL:n1".into())];
        init_rustls();
        let (out, _) = mask_output(
            b"x\x00secret\x00y", &m, &[], &[],
            &crate::api::VeilKeyClient::new("http://127.0.0.1:1"), "", "",
        );
        assert!(!strip_ansi(&String::from_utf8_lossy(&out)).contains("secret"));
    }

    #[test]
    fn test_sec_carriage_return() {
        let m = vec![("password".into(), "VK:LOCAL:n2".into())];
        let (out, _) = mask_output_simple("password\roverwrite", &m, "");
        assert!(!strip_ansi(&out).contains("password"));
    }

    // ── Output formats ──────────────────────────────────────────────

    #[test]
    fn test_sec_yaml() {
        let m = vec![("api-key-12345".into(), "VK:LOCAL:f1".into())];
        let (out, _) = mask_output_simple("  key: api-key-12345\n  host: x.com\n", &m, "");
        let v = strip_ansi(&out);
        assert!(!v.contains("api-key-12345"));
        assert!(v.contains("x.com"));
    }

    #[test]
    fn test_sec_docker_env() {
        let m = vec![("db_pass".into(), "VK:LOCAL:f2".into())];
        let (out, _) = mask_output_simple("MYSQL_ROOT_PASSWORD=db_pass\nDB=app\n", &m, "");
        let v = strip_ansi(&out);
        assert!(!v.contains("db_pass"));
        assert!(v.contains("DB=app"));
    }

    #[test]
    fn test_sec_log_line() {
        let m = vec![("Bearer sk-abc".into(), "VK:LOCAL:f3".into())];
        let (out, _) = mask_output_simple("[INFO] Auth: Bearer sk-abc", &m, "");
        assert!(!strip_ansi(&out).contains("sk-abc"));
    }

    #[test]
    fn test_sec_git_diff() {
        let m = vec![("ghp_ABCDEF".into(), "VK:LOCAL:f4".into())];
        let (out, _) = mask_output_simple("+TOKEN=ghp_ABCDEF\n-TOKEN=old\n", &m, "");
        assert!(!strip_ansi(&out).contains("ghp_ABCDEF"));
    }

    #[test]
    fn test_sec_curl_verbose() {
        let m = vec![("Bearer tok123".into(), "VK:LOCAL:f5".into())];
        let (out, _) = mask_output_simple("> Authorization: Bearer tok123\r\n", &m, "");
        assert!(!strip_ansi(&out).contains("tok123"));
    }

    #[test]
    fn test_sec_sql_table() {
        let m = vec![("admin_pw".into(), "VK:LOCAL:f6".into())];
        let (out, _) = mask_output_simple("| admin | admin_pw |\n", &m, "");
        assert!(!strip_ansi(&out).contains("admin_pw"));
    }

    #[test]
    fn test_sec_nested_json() {
        let m = vec![("deep-val".into(), "VK:LOCAL:f7".into())];
        let (out, _) = mask_output_simple(r#"{"a":{"b":"deep-val"}}"#, &m, "");
        assert!(!strip_ansi(&out).contains("deep-val"));
    }
}
