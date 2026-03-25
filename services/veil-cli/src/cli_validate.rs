//! Argument validation helpers for CLI commands.
//! Extracted from binary main() to allow unit testing.

/// Validate resolve command arguments.
/// Returns Ok(vk_ref) or Err(message).
pub fn validate_resolve_args(args: &[String], is_tty: bool) -> Result<String, String> {
    if !is_tty {
        return Err(
            "[veilkey] resolve requires an interactive terminal (blocked in pipes)".to_string(),
        );
    }
    match args.first() {
        Some(vk_ref) if !vk_ref.is_empty() => Ok(vk_ref.clone()),
        _ => Err("Usage: veilkey resolve <VK:ref>".to_string()),
    }
}

/// Validate function command arguments.
/// Returns Ok((subcmd, optional_name)) or Err(message).
pub fn validate_function_args(args: &[String]) -> Result<(String, Option<String>), String> {
    let subcmd = args
        .first()
        .ok_or_else(|| "Usage: veilkey function <list|add|remove> [name]".to_string())?;

    match subcmd.as_str() {
        "list" => Ok(("list".to_string(), None)),
        "add" => {
            let name = args
                .get(1)
                .ok_or_else(|| "Usage: veilkey function add <name>".to_string())?;
            if name.is_empty() {
                return Err("Usage: veilkey function add <name>".to_string());
            }
            Ok(("add".to_string(), Some(name.clone())))
        }
        "remove" => {
            let name = args
                .get(1)
                .ok_or_else(|| "Usage: veilkey function remove <name>".to_string())?;
            if name.is_empty() {
                return Err("Usage: veilkey function remove <name>".to_string());
            }
            Ok(("remove".to_string(), Some(name.clone())))
        }
        unknown => Err(format!(
            "Unknown function subcommand: {}\nUsage: veilkey function <list|add|remove> [name]",
            unknown
        )),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ── resolve ──────────────────────────────────────────────────────

    #[test]
    fn resolve_rejects_non_tty() {
        let args = vec!["VK:LOCAL:abc12345".to_string()];
        let result = validate_resolve_args(&args, false);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("interactive terminal"));
    }

    #[test]
    fn resolve_rejects_empty_args() {
        let args: Vec<String> = vec![];
        let result = validate_resolve_args(&args, true);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Usage"));
    }

    #[test]
    fn resolve_accepts_valid_ref() {
        let args = vec!["VK:LOCAL:abc12345".to_string()];
        let result = validate_resolve_args(&args, true);
        assert_eq!(result.unwrap(), "VK:LOCAL:abc12345");
    }

    #[test]
    fn resolve_accepts_temp_ref() {
        let args = vec!["VK:TEMP:deadbeef".to_string()];
        let result = validate_resolve_args(&args, true);
        assert_eq!(result.unwrap(), "VK:TEMP:deadbeef");
    }

    // ── function ─────────────────────────────────────────────────────

    #[test]
    fn function_rejects_no_subcmd() {
        let args: Vec<String> = vec![];
        let result = validate_function_args(&args);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Usage"));
    }

    #[test]
    fn function_list_no_name_required() {
        let args = vec!["list".to_string()];
        let (subcmd, name) = validate_function_args(&args).unwrap();
        assert_eq!(subcmd, "list");
        assert!(name.is_none());
    }

    #[test]
    fn function_add_requires_name() {
        let args = vec!["add".to_string()];
        let result = validate_function_args(&args);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Usage"));
    }

    #[test]
    fn function_add_with_name() {
        let args = vec!["add".to_string(), "my-func".to_string()];
        let (subcmd, name) = validate_function_args(&args).unwrap();
        assert_eq!(subcmd, "add");
        assert_eq!(name.unwrap(), "my-func");
    }

    #[test]
    fn function_remove_requires_name() {
        let args = vec!["remove".to_string()];
        let result = validate_function_args(&args);
        assert!(result.is_err());
    }

    #[test]
    fn function_remove_with_name() {
        let args = vec!["remove".to_string(), "old-func".to_string()];
        let (subcmd, name) = validate_function_args(&args).unwrap();
        assert_eq!(subcmd, "remove");
        assert_eq!(name.unwrap(), "old-func");
    }

    #[test]
    fn function_rejects_unknown_subcmd() {
        let args = vec!["deploy".to_string()];
        let result = validate_function_args(&args);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Unknown function subcommand"));
    }

    #[test]
    fn function_add_empty_name_rejected() {
        let args = vec!["add".to_string(), "".to_string()];
        let result = validate_function_args(&args);
        assert!(result.is_err());
    }

    // ── resolve: edge cases ─────────────────────────────────────

    #[test]
    fn resolve_empty_string_ref_rejected() {
        let args = vec!["".to_string()];
        let result = validate_resolve_args(&args, true);
        assert!(result.is_err());
    }

    #[test]
    fn resolve_whitespace_ref_accepted() {
        // Whitespace ref is technically valid (server will reject)
        let args = vec!["  ".to_string()];
        let result = validate_resolve_args(&args, true);
        assert!(result.is_ok()); // validation passes, server rejects
    }

    #[test]
    fn resolve_short_hash_ref() {
        let args = vec!["VK:LOCAL:ab".to_string()];
        assert!(validate_resolve_args(&args, true).is_ok());
    }

    #[test]
    fn resolve_long_hash_ref() {
        let args = vec!["VK:LOCAL:abcdef1234567890abcdef1234567890".to_string()];
        assert!(validate_resolve_args(&args, true).is_ok());
    }

    #[test]
    fn resolve_external_ref() {
        let args = vec!["VK:EXTERNAL:abc12345".to_string()];
        assert!(validate_resolve_args(&args, true).is_ok());
    }

    #[test]
    fn resolve_bare_hash() {
        let args = vec!["abc12345".to_string()];
        assert!(validate_resolve_args(&args, true).is_ok());
    }

    #[test]
    fn resolve_ignores_extra_args() {
        // Only first arg is used
        let args = vec!["VK:LOCAL:aaa".to_string(), "extra".to_string()];
        assert_eq!(validate_resolve_args(&args, true).unwrap(), "VK:LOCAL:aaa");
    }

    #[test]
    fn resolve_special_chars_in_ref() {
        let args = vec!["VK:LOCAL:abc+def/ghi=".to_string()];
        assert!(validate_resolve_args(&args, true).is_ok());
    }

    #[test]
    fn resolve_tty_true_and_false_both_tested() {
        let args = vec!["VK:LOCAL:abc".to_string()];
        assert!(validate_resolve_args(&args, true).is_ok());
        assert!(validate_resolve_args(&args, false).is_err());
    }

    // ── function: edge cases ────────────────────────────────────

    #[test]
    fn function_remove_empty_name_rejected() {
        let args = vec!["remove".to_string(), "".to_string()];
        assert!(validate_function_args(&args).is_err());
    }

    #[test]
    fn function_list_ignores_extra_args() {
        let args = vec!["list".to_string(), "ignored".to_string()];
        let (subcmd, name) = validate_function_args(&args).unwrap();
        assert_eq!(subcmd, "list");
        assert!(name.is_none());
    }

    #[test]
    fn function_add_name_with_special_chars() {
        let args = vec!["add".to_string(), "my-func_v2.0".to_string()];
        let (_, name) = validate_function_args(&args).unwrap();
        assert_eq!(name.unwrap(), "my-func_v2.0");
    }

    #[test]
    fn function_add_name_with_spaces() {
        let args = vec!["add".to_string(), "my func".to_string()];
        let (_, name) = validate_function_args(&args).unwrap();
        assert_eq!(name.unwrap(), "my func");
    }

    #[test]
    fn function_add_ignores_third_arg() {
        let args = vec!["add".to_string(), "func1".to_string(), "extra".to_string()];
        let (subcmd, name) = validate_function_args(&args).unwrap();
        assert_eq!(subcmd, "add");
        assert_eq!(name.unwrap(), "func1");
    }

    #[test]
    fn function_all_valid_subcmds() {
        for cmd in ["list", "add", "remove"] {
            let mut args = vec![cmd.to_string()];
            if cmd != "list" {
                args.push("name".to_string());
            }
            assert!(validate_function_args(&args).is_ok(), "failed for {}", cmd);
        }
    }

    #[test]
    fn function_case_sensitive() {
        // "List" (capital) is not valid — only lowercase
        let args = vec!["List".to_string()];
        assert!(validate_function_args(&args).is_err());
    }

    #[test]
    fn function_add_unicode_name() {
        let args = vec!["add".to_string(), "함수-이름".to_string()];
        let (_, name) = validate_function_args(&args).unwrap();
        assert_eq!(name.unwrap(), "함수-이름");
    }

    #[test]
    fn function_remove_unicode_name() {
        let args = vec!["remove".to_string(), "テスト".to_string()];
        let (_, name) = validate_function_args(&args).unwrap();
        assert_eq!(name.unwrap(), "テスト");
    }

    // ── domain: security guarantees ─────────────────────────────

    #[test]
    fn domain_resolve_never_works_in_pipe() {
        // This is a security invariant: resolve must NEVER work without TTY
        for ref_str in ["VK:LOCAL:abc", "VK:TEMP:def", "abc12345", "anything"] {
            let args = vec![ref_str.to_string()];
            assert!(
                validate_resolve_args(&args, false).is_err(),
                "resolve must reject non-TTY for ref: {}",
                ref_str
            );
        }
    }

    #[test]
    fn domain_function_unknown_subcmds_rejected() {
        for bad in [
            "delete", "update", "get", "run", "exec", "ls", "rm", "create",
        ] {
            let args = vec![bad.to_string()];
            assert!(
                validate_function_args(&args).is_err(),
                "must reject unknown subcmd: {}",
                bad
            );
        }
    }
}
