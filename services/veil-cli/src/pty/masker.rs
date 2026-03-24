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

    for (plaintext, ve_ref) in ve_map {
        if !plaintext.is_empty() && s.contains(plaintext.as_str()) {
            s = s.replace(plaintext.as_str(), &colorize_ve_ref(plaintext, ve_ref));
        }
    }

    s.into_bytes()
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
}

