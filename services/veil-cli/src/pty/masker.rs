use crate::api::VeilKeyClient;
use crate::config::CompiledPattern;

const BOLD: &str = "\x1b[1m";
const CYAN: &str = "\x1b[36m";
const RED: &str = "\x1b[31m";
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

/// VE: colorize original value in green — same text, different color.
pub fn colorize_ve_ref(original: &str, _ve_ref: &str) -> String {
    format!("{}{}{}", GREEN, original, RESET)
}

/// Replace a secret with a colorized VK ref, padded to EXACTLY the original width.
/// This ensures no surrounding text shifts position and no characters leak.
/// If the ref is longer than the original, we truncate the visible ref to fit.
pub fn padded_colorize_ref(vk_ref: &str, original_len: usize) -> String {
    if original_len == 0 {
        return String::new();
    }
    let ref_visible_len = vk_ref.chars().count();
    if ref_visible_len <= original_len {
        // Ref fits — pad with spaces to fill the original width
        let colored = colorize_ref(vk_ref);
        let pad = original_len - ref_visible_len;
        if pad > 0 {
            format!("{}{}", colored, " ".repeat(pad))
        } else {
            colored
        }
    } else {
        // Ref is longer than original — truncate to fit exactly
        let truncated: String = vk_ref.chars().take(original_len).collect();
        colorize_ref(&truncated)
    }
}

// Alt-screen detection sequences (vim, less, htop, etc.)
const ALT_SCREEN_ENABLE: &[u8] = b"\x1b[?1049h";
const ALT_SCREEN_DISABLE: &[u8] = b"\x1b[?1049l";

/// Check if data contains alt-screen toggle and return new state.
pub fn detect_alt_screen(data: &[u8], current: bool) -> bool {
    // Last occurrence wins (multiple toggles in one chunk)
    let enable_pos = data.windows(ALT_SCREEN_ENABLE.len())
        .rposition(|w| w == ALT_SCREEN_ENABLE);
    let disable_pos = data.windows(ALT_SCREEN_DISABLE.len())
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
    // is issued to the API (registered for future mask_map inclusion).
    // The actual output masking uses only new_text below.
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
            // We don't replace here; the output masking below handles replacement.
            let _ = client.issue(secret);
        }
    }

    // Output masking: apply replacements on new_text only (tail was already emitted).
    // The combined pre-scan above already issued split secrets to the API.
    let mut output = new_text.clone();
    let mut output_had_replacement = false;
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
            if let Ok(ref_canonical) = client.issue(secret) {
                output = output.replace(secret, &padded_colorize_ref(&ref_canonical, secret.len()));
                output_had_replacement = true;
            }
        }
    }
    for (plaintext, ve_ref) in ve_map {
        if !plaintext.is_empty() && output.contains(plaintext.as_str()) {
            output = output.replace(plaintext.as_str(), &colorize_ve_ref(plaintext, ve_ref));
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
        format!("{}{}", &plain_tail[plain_tail.ceil_char_boundary(plain_tail.len().saturating_sub(PLAIN_TAIL_SIZE / 2))..], &new_text[start..])
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
        let mask_map = vec![
            ("SuperSecret123".to_string(), "VK:LOCAL:aaa11111".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len(), "line width must be preserved");
        assert!(result.contains("@db.example.com:3306/mydb"), "host must be intact");
        assert!(result.contains("admin:"), "username must be intact");
        assert!(!result.contains("SuperSecret"), "password must not leak");
    }

    #[test]
    fn test_mask_connection_string_with_at_in_password() {
        // Password contains @ — the FULL password must be in mask_map
        let input = "postgres://user:p@ss!word@host:5432";
        let mask_map = vec![
            ("p@ss!word".to_string(), "VK:LOCAL:bbb22222".to_string()),
        ];
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
            ("secret-api-key-value".to_string(), "VK:LOCAL:aaa".to_string()),
            ("db-password-here".to_string(), "VK:LOCAL:bbb".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("secret-api"), "first secret must not leak");
        assert!(!result.contains("db-password"), "second secret must not leak");
        assert!(result.contains("API_KEY="), "key names must survive");
        assert!(result.contains("DB_PASS="), "key names must survive");
    }

    #[test]
    fn test_mask_same_secret_twice() {
        let input = "first=MyPassword second=MyPassword";
        let mask_map = vec![
            ("MyPassword".to_string(), "VK:LOCAL:ccc".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        // Both occurrences must be masked
        assert!(!result.contains("MyPassword"));
    }

    // ── Secret at boundaries ────────────────────────────────────────

    #[test]
    fn test_mask_secret_at_line_start() {
        let input = "SuperSecret123 is exposed";
        let mask_map = vec![
            ("SuperSecret123".to_string(), "VK:LOCAL:ddd".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("SuperSecret"));
        assert!(result.contains(" is exposed"));
    }

    #[test]
    fn test_mask_secret_at_line_end() {
        let input = "password is SuperSecret123";
        let mask_map = vec![
            ("SuperSecret123".to_string(), "VK:LOCAL:eee".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("SuperSecret"));
        assert!(result.contains("password is "));
    }

    #[test]
    fn test_mask_secret_is_entire_line() {
        let input = "SuperSecret123";
        let mask_map = vec![
            ("SuperSecret123".to_string(), "VK:LOCAL:fff".to_string()),
        ];
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
        assert!(!result.contains("api-key"), "the full match must be used, not partial");
    }

    // ── Special characters in secrets ───────────────────────────────

    #[test]
    fn test_mask_secret_with_special_chars() {
        let input = "PASS=p@$$w0rd!#%^&*()";
        let mask_map = vec![
            ("p@$$w0rd!#%^&*()".to_string(), "VK:LOCAL:spec1".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("p@$$"));
        assert!(result.contains("PASS="));
    }

    #[test]
    fn test_mask_secret_with_quotes() {
        let input = r#"export SECRET="my-quoted-secret""#;
        let mask_map = vec![
            ("my-quoted-secret".to_string(), "VK:LOCAL:quot1".to_string()),
        ];
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
        let mask_map = vec![
            ("sk-1234567890abcdef".to_string(), "VK:LOCAL:json1".to_string()),
        ];
        let result = simulate_mask(input, &mask_map);
        assert_eq!(result.len(), input.len());
        assert!(!result.contains("sk-1234567890"));
        assert!(result.contains("example.com"));
    }

    // ── env var output ──────────────────────────────────────────────

    #[test]
    fn test_mask_env_export_line() {
        let input = "export DATABASE_URL=postgres://admin:hunter2@db:5432/prod";
        let mask_map = vec![
            ("hunter2".to_string(), "VK:LOCAL:env1".to_string()),
        ];
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
        let mask_map = vec![
            ("SuperSecret".to_string(), "VK:LOCAL:nope".to_string()),
        ];
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
            let secret: String = (0..secret_len).map(|i| (b'a' + (i % 26) as u8) as char).collect();
            let input = format!("prefix:{}:suffix", secret);
            let mask_map = vec![
                (secret.clone(), "VK:LOCAL:6da25530".to_string()),
            ];
            let result = simulate_mask(&input, &mask_map);
            assert_eq!(
                result.len(), input.len(),
                "width mismatch for secret_len={}: input=[{}] result=[{}]",
                secret_len, input, result
            );
            assert!(!result.contains(&secret), "secret leaked for len={}", secret_len);
            assert!(result.contains(":suffix"), "suffix missing for len={}", secret_len);
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
}

