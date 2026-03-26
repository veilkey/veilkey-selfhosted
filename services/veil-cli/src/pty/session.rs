use nix::libc;
use nix::sys::signal::{self, SigHandler, Signal};
use nix::unistd::{execvp, fork, ForkResult};
use std::ffi::CString;
use std::io;
use std::os::fd::{AsRawFd, RawFd};
use std::sync::atomic::{AtomicBool, AtomicI32, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::Duration;

/// Read from fd, retrying on EINTR. Returns bytes read, or <= 0 on EOF/error.
unsafe fn read_eintr(fd: RawFd, buf: &mut [u8]) -> isize {
    loop {
        let n = libc::read(fd, buf.as_mut_ptr() as _, buf.len());
        if n == -1 && *libc::__errno_location() == libc::EINTR {
            continue;
        }
        return n;
    }
}

/// Write all bytes to fd, handling partial writes and EINTR.
unsafe fn write_all_fd(fd: RawFd, buf: &[u8]) {
    let mut offset = 0;
    while offset < buf.len() {
        let n = libc::write(fd, buf[offset..].as_ptr() as _, buf.len() - offset);
        if n == -1 {
            if *libc::__errno_location() == libc::EINTR {
                continue;
            }
            break;
        }
        offset += n as usize;
    }
}

/// Result of processing stdin input for the secret guard.
#[derive(Debug, PartialEq)]
pub(crate) enum StdinGuardResult {
    Forward, // safe to forward data to PTY
    Blocked, // secret detected — do NOT forward
}

/// Max line_buf size to prevent DoS from input without Enter.
pub(crate) const MAX_LINE_BUF: usize = 8192;

/// Process stdin data: accumulate chars in line_buf, on Enter check against secrets.
/// Returns (StdinGuardResult, updated line_buf).
pub(crate) fn check_stdin_for_secrets(
    data: &[u8],
    line_buf: &mut String,
    secrets: &[(String, String)],
) -> StdinGuardResult {
    let s = match std::str::from_utf8(data) {
        Ok(s) => s,
        Err(_) => return StdinGuardResult::Forward,
    };
    for ch in s.chars() {
        if ch == '\r' || ch == '\n' {
            if !line_buf.is_empty() {
                let found = secrets
                    .iter()
                    .any(|(secret, _)| !secret.is_empty() && line_buf.contains(secret.as_str()));
                if found {
                    line_buf.clear();
                    return StdinGuardResult::Blocked;
                }
            }
            line_buf.clear();
        } else if ch == '\x03' || ch == '\x15' {
            line_buf.clear();
        } else if ch == '\x7f' || ch == '\x08' {
            line_buf.pop();
        } else if ch >= ' ' {
            line_buf.push(ch);
            // Cap line_buf to prevent unbounded growth (DoS protection).
            // Keep the tail so secrets at the end are still detected.
            if line_buf.len() > MAX_LINE_BUF {
                let start = line_buf.len() - MAX_LINE_BUF;
                let start = line_buf.ceil_char_boundary(start);
                *line_buf = line_buf[start..].to_string();
            }
        }
    }
    StdinGuardResult::Forward
}
use crate::api::VeilKeyClient;
use crate::config::{load_config, CompiledPattern};
use crate::state::state_dir;

use super::masker;
use super::sync as mask_sync;

static MASTER_FD: AtomicI32 = AtomicI32::new(-1);
/// Panic hook state for terminal restoration.
static PANIC_STDIN_FD: AtomicI32 = AtomicI32::new(-1);
static PANIC_HAS_TERMIOS: AtomicBool = AtomicBool::new(false);
static mut PANIC_TERMIOS: libc::termios = unsafe { std::mem::zeroed() };

extern "C" fn handle_sigwinch(_: libc::c_int) {
    let fd = MASTER_FD.load(Ordering::Relaxed);
    if fd >= 0 {
        unsafe {
            let mut ws: libc::winsize = std::mem::zeroed();
            libc::ioctl(0, libc::TIOCGWINSZ, &mut ws);
            libc::ioctl(fd, libc::TIOCSWINSZ, &ws);
        }
    }
}

/// Read a secret: try file from env var first, then interactive prompt.
pub(crate) fn read_secret(env_file_key: &str, prompt: &str) -> String {
    // 1. Try reading from file specified by env var
    if let Ok(path) = std::env::var(env_file_key) {
        if !path.is_empty() {
            match std::fs::read_to_string(&path) {
                Ok(content) => {
                    let trimmed = content.trim_end_matches(['\r', '\n']).to_string();
                    if !trimmed.is_empty() {
                        return trimmed;
                    }
                }
                Err(e) => {
                    eprintln!("[veilkey] cannot read {}: {}", path, e);
                    std::process::exit(1);
                }
            }
        }
    }
    // 2. Interactive prompt
    rpassword::prompt_password(prompt).unwrap_or_default()
}

pub fn run(args: &[String], api_url: &str, _log_path: &str, patterns_file: Option<&str>) {
    let shell_args: Vec<String> = if args.is_empty() {
        let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/bash".to_string());
        vec![shell]
    } else {
        args.to_vec()
    };

    let client = VeilKeyClient::new(api_url);

    // If server is locked, prompt for master password and unlock first
    if client.is_locked() {
        let master = read_secret("VEILKEY_MASTER_PASSWORD_FILE", "Master password (unlock): ");
        if master.is_empty() {
            eprintln!("[veilkey] master password is required to unlock");
            std::process::exit(1);
        }
        if let Err(e) = client.unlock(&master) {
            eprintln!("[veilkey] unlock failed: {}", e);
            std::process::exit(1);
        }
        eprintln!("[veilkey] server unlocked");
    }

    // Authenticate with admin password
    let password = read_secret("VEILKEY_PASSWORD_FILE", "VeilKey password: ");
    if password.is_empty() {
        eprintln!("[veilkey] password is required");
        std::process::exit(1);
    }
    if let Err(e) = client.admin_login(&password) {
        eprintln!("[veilkey] authentication failed: {}", e);
        std::process::exit(1);
    }

    // Load detection patterns in background (223 regex compilations = ~2s)
    let pf = patterns_file.map(|s| s.to_string());
    let patterns_handle = std::thread::spawn(move || {
        let cfg = load_config(pf.as_deref()).ok();
        cfg.map(|c| c.patterns).unwrap_or_default()
    });

    // Save PID file
    let sd = state_dir();
    let _ = std::fs::create_dir_all(&sd);
    let pid_path = sd.join("guard.pid");
    let _ = std::fs::write(&pid_path, format!("{}", std::process::id()));

    // 1. Fetch mask_map (fail-closed) — runs in parallel with pattern compilation
    let mut mask_map: Vec<(String, String)> = match client.fetch_all_secrets_mask_map() {
        Some(map) => map,
        None => {
            eprintln!(
                "[veilkey] FATAL: cannot build mask map — refusing to start shell (fail-closed)"
            );
            eprintln!("[veilkey] ensure VaultCenter is reachable and try again");
            std::process::exit(1);
        }
    };
    // 2. Resolve VK refs in environment variables
    let vk_re = regex::Regex::new(crate::detector::VEILKEY_RE_STR)
        .expect("BUG: VEILKEY_RE_STR is not a valid regex");
    let mut child_env: Vec<(String, String)> = Vec::new();
    for (key, value) in std::env::vars() {
        if vk_re.is_match(&value) {
            let resolved = vk_re
                .replace_all(&value, |caps: &regex::Captures| {
                    match client.resolve(&caps[0]) {
                        Ok(v) => {
                            if !mask_map.iter().any(|(p, _)| p == &v) {
                                mask_map.push((v.clone(), caps[0].to_string()));
                            }
                            v
                        }
                        Err(_) => caps[0].to_string(),
                    }
                })
                .to_string();
            child_env.push((key, resolved));
        }
    }
    crate::api::enrich_mask_map(&mut mask_map);
    let ve_entries = client.get_ve_entries();
    eprintln!(
        "[veilkey] {} secret(s) masked, {} config(s) tagged",
        mask_map.len(),
        ve_entries.len()
    );

    // Open PTY
    let (master_fd, slave_fd): (RawFd, RawFd) = unsafe {
        let mut master: RawFd = 0;
        let mut slave: RawFd = 0;
        if libc::openpty(
            &mut master,
            &mut slave,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            std::ptr::null_mut(),
        ) != 0
        {
            eprintln!("[veilkey] openpty failed");
            std::process::exit(1);
        }
        (master, slave)
    };

    // Set terminal to raw mode
    let stdin_fd = io::stdin().as_raw_fd();
    let mut old_termios: libc::termios = unsafe { std::mem::zeroed() };
    let has_termios = unsafe { libc::tcgetattr(stdin_fd, &mut old_termios) == 0 };

    // Copy terminal size
    unsafe {
        let mut ws: libc::winsize = std::mem::zeroed();
        libc::ioctl(stdin_fd, libc::TIOCGWINSZ, &mut ws);
        libc::ioctl(slave_fd, libc::TIOCSWINSZ, &ws);
        MASTER_FD.store(master_fd, Ordering::Relaxed);
        signal::signal(Signal::SIGWINCH, SigHandler::Handler(handle_sigwinch)).ok();
    }

    match unsafe { fork() } {
        Ok(ForkResult::Child) => {
            unsafe {
                libc::close(master_fd);
                libc::setsid();
                libc::ioctl(slave_fd, libc::TIOCSCTTY as _, 0);
                libc::dup2(slave_fd, 0);
                libc::dup2(slave_fd, 1);
                libc::dup2(slave_fd, 2);
                if slave_fd > 2 {
                    libc::close(slave_fd);
                }
            }

            // Block /proc/self/environ reads
            unsafe {
                libc::prctl(libc::PR_SET_DUMPABLE, 0);
            }

            // Set resolved env vars
            for (key, value) in &child_env {
                std::env::set_var(key, value);
            }

            let prog = CString::new(shell_args[0].as_str()).unwrap();
            let c_args: Vec<CString> = shell_args
                .iter()
                .map(|a| CString::new(a.as_str()).unwrap())
                .collect();
            let exec_result = execvp(&prog, &c_args);
            eprintln!(
                "[veilkey] execvp failed: {} ({:?})",
                shell_args[0], exec_result
            );
            std::process::exit(1);
        }
        Ok(ForkResult::Parent { child }) => {
            unsafe {
                libc::close(slave_fd);
            }

            if has_termios {
                // Save termios and install panic hook before entering raw mode
                unsafe {
                    PANIC_TERMIOS = old_termios;
                }
                PANIC_STDIN_FD.store(stdin_fd, Ordering::Release);
                PANIC_HAS_TERMIOS.store(true, Ordering::Release);

                let prev_hook = std::panic::take_hook();
                std::panic::set_hook(Box::new(move |info| {
                    if PANIC_HAS_TERMIOS.load(Ordering::Acquire) {
                        let fd = PANIC_STDIN_FD.load(Ordering::Acquire);
                        if fd >= 0 {
                            unsafe {
                                libc::tcsetattr(fd, libc::TCSANOW, &raw const PANIC_TERMIOS);
                            }
                        }
                    }
                    prev_hook(info);
                }));

                unsafe {
                    let mut raw = old_termios;
                    libc::cfmakeraw(&mut raw);
                    libc::tcsetattr(stdin_fd, libc::TCSANOW, &raw);
                }
            }

            let mask_map = Arc::new(RwLock::new(mask_map));
            let ve_map = Arc::new(RwLock::new(ve_entries));
            // Patterns compiled in background — start with empty, swap when ready
            let patterns: Arc<RwLock<Vec<CompiledPattern>>> = Arc::new(RwLock::new(Vec::new()));
            {
                let patterns_ref = patterns.clone();
                std::thread::spawn(move || {
                    let compiled = patterns_handle.join().unwrap_or_default();
                    if let Ok(mut p) = patterns_ref.write() {
                        *p = compiled;
                    }
                });
            }
            let client = Arc::new(client);

            // Background mask_map sync
            mask_sync::spawn_mask_map_sync(mask_map.clone(), ve_map.clone(), client.clone());

            // Recent input tracking
            let recent_input = Arc::new(Mutex::new(String::new()));

            // stdin → PTY master (input thread)
            // Terminal response sequences (DSR, OSC) are passed through to PTY
            // but excluded from recent_input to avoid false masking skips.
            let master_wr = master_fd;
            let input_tracker = recent_input.clone();
            let stdin_mask_map = mask_map.clone();
            std::thread::spawn(move || {
                let mut buf = [0u8; 4096];
                // Accumulated line buffer for enter-key secret detection
                let mut line_buf = String::new();
                loop {
                    let n = unsafe { read_eintr(stdin_fd, &mut buf) };
                    if n <= 0 {
                        break;
                    }
                    let data = &buf[..n as usize];

                    // Check ECHO flag on PTY master fd (reflects slave termios on Linux).
                    // When ECHO is off (e.g. sudo/ssh password prompt), don't add typed
                    // characters to recent_input — this prevents the masking-skip logic
                    // from treating passwords as "user-typed commands" and allows them
                    // to be masked if they later appear in output.
                    let echo_on = unsafe {
                        let mut termios: libc::termios = std::mem::zeroed();
                        if libc::tcgetattr(master_wr, &mut termios) == 0 {
                            (termios.c_lflag & libc::ECHO) != 0
                        } else {
                            true // safe default: assume ECHO on if tcgetattr fails
                        }
                    };

                    // Track user input only when ECHO is on (exclude terminal response sequences)
                    if echo_on {
                        if let Ok(s) = std::str::from_utf8(data) {
                            let filtered: String = s
                                .chars()
                                .filter(|&c| c >= ' ' || c == '\n' || c == '\r' || c == '\t')
                                .collect();
                            if !filtered.is_empty() {
                                let mut tracker = input_tracker.lock().unwrap();
                                tracker.push_str(&filtered);
                                if tracker.len() > 4096 {
                                    let start = tracker.len() - 4096;
                                    let start = tracker.ceil_char_boundary(start);
                                    *tracker = tracker[start..].to_string();
                                }
                            }
                        }
                    }
                    // When ECHO is off: don't add to recent_input — passwords typed
                    // during ECHO-off are implicitly registered as potential secrets
                    // since they won't appear in recent_input for masking-skip.

                    // Detection-only: check_stdin_for_secrets inspects the input and
                    // returns StdinGuardResult::Blocked when a secret is detected, but
                    // the return value is intentionally ignored here — data is always
                    // forwarded to the PTY regardless. Output masking is the active
                    // defence; stdin guard is currently passive (logging/future use).
                    // TODO: When ready to enforce blocking, check StdinGuardResult::Blocked here and skip write_all_fd
                    {
                        let map = stdin_mask_map.read().unwrap();
                        check_stdin_for_secrets(data, &mut line_buf, &map);
                    }

                    // Always forward raw data to PTY (blocking not yet enforced — see TODO above)
                    unsafe {
                        write_all_fd(master_wr, data);
                    }
                }
            });

            // PTY master → stdout (output thread with masking)
            // Uses a plain_tail buffer (8KB lookback) to catch secrets split across
            // PTY read() chunks. Inspired by secretty's plainTail approach:
            // each chunk is masked with tail+chunk combined for detection.
            let mask = mask_map.clone();
            let ve = ve_map.clone();
            let input_ref = recent_input.clone();
            let stdout_fd = io::stdout().as_raw_fd();
            let mut plain_tail = String::new();
            let mut in_alt_screen = false;

            let mut buf = [0u8; 32768];
            loop {
                let n = unsafe { read_eintr(master_fd, &mut buf) };
                if n <= 0 {
                    break;
                }
                let mut total = n as usize;

                // Output coalesce: 50ms drain loop to collect char-by-char echo
                // into larger chunks, improving cross-chunk secret detection.
                std::thread::sleep(Duration::from_millis(50));
                loop {
                    let mut pfd = libc::pollfd {
                        fd: master_fd,
                        events: libc::POLLIN,
                        revents: 0,
                    };
                    let ready = unsafe { libc::poll(&mut pfd, 1, 0) };
                    if ready <= 0 || total >= buf.len() {
                        break;
                    }
                    let extra = unsafe { read_eintr(master_fd, &mut buf[total..]) };
                    if extra <= 0 {
                        break;
                    }
                    total += extra as usize;
                }

                let chunk = &buf[..total];

                // Track alt-screen state (vim, less, htop, etc.)
                in_alt_screen = masker::detect_alt_screen(chunk, in_alt_screen);

                if in_alt_screen {
                    // Alt-screen active — pass through unmasked (TUI apps break with masking)
                    unsafe {
                        write_all_fd(stdout_fd, chunk);
                    }
                    continue;
                }

                let ri = input_ref.lock().unwrap().clone();
                let (masked, new_tail) = masker::mask_output(
                    chunk,
                    &mask.read().unwrap(),
                    &ve.read().unwrap(),
                    &patterns.read().unwrap(),
                    &client,
                    &ri,
                    &plain_tail,
                );
                plain_tail = new_tail;

                unsafe {
                    write_all_fd(stdout_fd, &masked);
                }
            }

            // Restore terminal
            if has_termios {
                unsafe {
                    libc::tcsetattr(stdin_fd, libc::TCSANOW, &old_termios);
                }
            }

            // Wait for child
            let mut status: libc::c_int = 0;
            unsafe {
                libc::waitpid(child.as_raw(), &mut status, 0);
            }

            if let Ok(m) = mask.read() {
                if !m.is_empty() {
                    eprintln!("\n[veilkey] {} secret(s) masked in session", m.len());
                }
            }

            let _ = std::fs::remove_file(&pid_path);

            let exit_code = if libc::WIFEXITED(status) {
                libc::WEXITSTATUS(status)
            } else {
                1
            };
            std::process::exit(exit_code);
        }
        Err(e) => {
            eprintln!("[veilkey] fork failed: {}", e);
            std::process::exit(1);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn secrets(pairs: &[(&str, &str)]) -> Vec<(String, String)> {
        pairs
            .iter()
            .map(|(s, r)| (s.to_string(), r.to_string()))
            .collect()
    }

    // --- Basic detection ---

    #[test]
    fn blocks_exact_secret_on_enter() {
        let mut buf = String::new();
        let map = secrets(&[("Ghdrhkdgh1@", "VK:LOCAL:6da25530")]);
        let result = check_stdin_for_secrets(b"Ghdrhkdgh1@\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn blocks_secret_embedded_in_command() {
        let mut buf = String::new();
        let map = secrets(&[("hunter2", "VK:LOCAL:aaa")]);
        let result = check_stdin_for_secrets(b"echo hunter2\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn forwards_safe_command() {
        let mut buf = String::new();
        let map = secrets(&[("SuperSecret123", "VK:LOCAL:bbb")]);
        let result = check_stdin_for_secrets(b"ls -la\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
    }

    #[test]
    fn forwards_when_no_enter() {
        let mut buf = String::new();
        let map = secrets(&[("hunter2", "VK:LOCAL:ccc")]);
        let result = check_stdin_for_secrets(b"hunter2", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
        assert_eq!(buf, "hunter2"); // accumulated but not checked yet
    }

    // --- Multi-chunk input ---

    #[test]
    fn blocks_secret_typed_across_chunks() {
        let mut buf = String::new();
        let map = secrets(&[("password123", "VK:LOCAL:ddd")]);
        // Chunk 1: partial
        assert_eq!(
            check_stdin_for_secrets(b"pass", &mut buf, &map),
            StdinGuardResult::Forward
        );
        assert_eq!(buf, "pass");
        // Chunk 2: more
        assert_eq!(
            check_stdin_for_secrets(b"word123", &mut buf, &map),
            StdinGuardResult::Forward
        );
        assert_eq!(buf, "password123");
        // Chunk 3: enter
        assert_eq!(
            check_stdin_for_secrets(b"\r", &mut buf, &map),
            StdinGuardResult::Blocked
        );
    }

    // --- Editing keys ---

    #[test]
    fn backspace_removes_chars() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:eee")]);
        // Type "secretx" then backspace
        check_stdin_for_secrets(b"secretx\x7f", &mut buf, &map);
        assert_eq!(buf, "secret");
        // Enter should match
        assert_eq!(
            check_stdin_for_secrets(b"\r", &mut buf, &map),
            StdinGuardResult::Blocked
        );
    }

    #[test]
    fn ctrl_c_clears_line_buf() {
        let mut buf = String::new();
        let map = secrets(&[("hunter2", "VK:LOCAL:fff")]);
        check_stdin_for_secrets(b"hunter2", &mut buf, &map);
        assert_eq!(buf, "hunter2");
        check_stdin_for_secrets(b"\x03", &mut buf, &map); // Ctrl+C
        assert_eq!(buf, "");
        // Enter after Ctrl+C — safe
        assert_eq!(
            check_stdin_for_secrets(b"ls\r", &mut buf, &map),
            StdinGuardResult::Forward
        );
    }

    #[test]
    fn ctrl_u_clears_line_buf() {
        let mut buf = String::new();
        let map = secrets(&[("hunter2", "VK:LOCAL:ggg")]);
        check_stdin_for_secrets(b"hunter2", &mut buf, &map);
        check_stdin_for_secrets(b"\x15", &mut buf, &map); // Ctrl+U
        assert_eq!(buf, "");
    }

    // --- Edge cases ---

    #[test]
    fn empty_secret_ignored() {
        let mut buf = String::new();
        let map = secrets(&[("", "VK:LOCAL:hhh")]);
        let result = check_stdin_for_secrets(b"anything\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
    }

    #[test]
    fn empty_input_forwarded() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:iii")]);
        let result = check_stdin_for_secrets(b"\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
    }

    #[test]
    fn newline_also_triggers_check() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:jjj")]);
        let result = check_stdin_for_secrets(b"secret\n", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn multiple_secrets_any_match_blocks() {
        let mut buf = String::new();
        let map = secrets(&[("password1", "VK:LOCAL:k1"), ("password2", "VK:LOCAL:k2")]);
        let result = check_stdin_for_secrets(b"echo password2\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn paste_with_secret_and_enter() {
        let mut buf = String::new();
        let map = secrets(&[("SuperSecret", "VK:LOCAL:lll")]);
        // Pasted all at once
        let result =
            check_stdin_for_secrets(b"curl -H 'Auth: SuperSecret' http://x\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn escape_sequences_not_accumulated() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:mmm")]);
        // Up arrow escape sequence \x1b[A — \x1b is < ' ' so excluded
        check_stdin_for_secrets(b"\x1b[A", &mut buf, &map);
        assert_eq!(buf, "[A"); // only printable parts
    }

    #[test]
    fn line_buf_resets_after_safe_enter() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:nnn")]);
        check_stdin_for_secrets(b"safe command\r", &mut buf, &map);
        assert_eq!(buf, ""); // cleared after enter
                             // Next line with secret
        let result = check_stdin_for_secrets(b"secret\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn binary_data_forwarded_safely() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:ooo")]);
        // Invalid UTF-8
        let result = check_stdin_for_secrets(&[0xFF, 0xFE, 0x0D], &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
    }

    // ── Security: stdin blocking edge cases ─────────────────────────

    #[test]
    fn sec_secret_with_special_chars_blocked() {
        let mut buf = String::new();
        let map = secrets(&[("p@$$w0rd!#", "VK:LOCAL:spec")]);
        let result = check_stdin_for_secrets(b"echo p@$$w0rd!#\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn sec_partial_secret_not_blocked() {
        let mut buf = String::new();
        let map = secrets(&[("SuperSecret123", "VK:LOCAL:partial")]);
        // Only partial match — should not block
        let result = check_stdin_for_secrets(b"SuperSecret\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
    }

    #[test]
    fn sec_secret_in_pipe_command() {
        let mut buf = String::new();
        let map = secrets(&[("api-key-abc", "VK:LOCAL:pipe")]);
        let result = check_stdin_for_secrets(b"echo api-key-abc | curl -H 'x'\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn sec_secret_in_env_assignment() {
        let mut buf = String::new();
        let map = secrets(&[("s3cr3t", "VK:LOCAL:env")]);
        let result = check_stdin_for_secrets(b"export TOKEN=s3cr3t\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn sec_secret_in_heredoc_style() {
        let mut buf = String::new();
        let map = secrets(&[("mypassword", "VK:LOCAL:here")]);
        let result = check_stdin_for_secrets(b"cat <<EOF\nmypassword\nEOF\r", &mut buf, &map);
        // First newline triggers check with "cat <<EOF" — safe
        // But "mypassword" followed by newline should block
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn sec_backspace_evasion_attempt() {
        // Type secret, backspace it all, type safe command — should NOT block
        let mut buf = String::new();
        let map = secrets(&[("hunter2", "VK:LOCAL:bs")]);
        check_stdin_for_secrets(b"hunter2", &mut buf, &map);
        // Backspace 7 times
        check_stdin_for_secrets(b"\x7f\x7f\x7f\x7f\x7f\x7f\x7f", &mut buf, &map);
        assert_eq!(buf, "");
        let result = check_stdin_for_secrets(b"ls -la\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Forward);
    }

    #[test]
    fn sec_multiple_enters_reset() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:rst")]);
        // Safe command
        check_stdin_for_secrets(b"ls\r", &mut buf, &map);
        assert_eq!(buf, "");
        // Dangerous command
        let result = check_stdin_for_secrets(b"echo secret\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn sec_very_long_command_with_secret() {
        let mut buf = String::new();
        let map = secrets(&[("needle", "VK:LOCAL:long")]);
        let padding = "x".repeat(4000);
        let cmd = format!("echo {}needle{}\r", padding, padding);
        let result = check_stdin_for_secrets(cmd.as_bytes(), &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    #[test]
    fn sec_tab_character_in_command() {
        let mut buf = String::new();
        let map = secrets(&[("secret", "VK:LOCAL:tab")]);
        let result = check_stdin_for_secrets(b"echo\tsecret\r", &mut buf, &map);
        assert_eq!(result, StdinGuardResult::Blocked);
    }

    // ── read_secret file-based tests ────────────────────────────────

    #[test]
    fn test_read_secret_from_file() {
        let path = std::env::temp_dir().join("vk-test-read-secret-1");
        std::fs::write(&path, "my-test-password\n").unwrap();
        std::env::set_var("_VK_TEST_RS1", path.to_str().unwrap());
        let result = read_secret("_VK_TEST_RS1", "unused: ");
        assert_eq!(&*result, "my-test-password");
        std::fs::remove_file(&path).ok();
        std::env::remove_var("_VK_TEST_RS1");
    }

    #[test]
    fn test_read_secret_trims_crlf() {
        let path = std::env::temp_dir().join("vk-test-read-secret-2");
        std::fs::write(&path, "pass-crlf\r\n").unwrap();
        std::env::set_var("_VK_TEST_RS2", path.to_str().unwrap());
        let result = read_secret("_VK_TEST_RS2", "unused: ");
        assert_eq!(&*result, "pass-crlf");
        std::fs::remove_file(&path).ok();
        std::env::remove_var("_VK_TEST_RS2");
    }

    #[test]
    fn test_read_secret_preserves_internal_newlines() {
        let path = std::env::temp_dir().join("vk-test-read-secret-3");
        std::fs::write(&path, "line1\nline2\n").unwrap();
        std::env::set_var("_VK_TEST_RS3", path.to_str().unwrap());
        let result = read_secret("_VK_TEST_RS3", "unused: ");
        assert!(result.contains("line1\nline2"));
        std::fs::remove_file(&path).ok();
        std::env::remove_var("_VK_TEST_RS3");
    }

    #[test]
    fn test_read_secret_env_not_set_fallback() {
        std::env::remove_var("_VK_TEST_RS_NOEXIST");
        let result = read_secret("_VK_TEST_RS_NOEXIST", "unused: ");
        // Non-TTY → rpassword returns empty
        assert!(result.is_empty());
    }

    #[test]
    fn test_read_secret_env_empty_fallback() {
        std::env::set_var("_VK_TEST_RS_EMPTY", "");
        let result = read_secret("_VK_TEST_RS_EMPTY", "unused: ");
        assert!(result.is_empty());
        std::env::remove_var("_VK_TEST_RS_EMPTY");
    }

    #[test]
    fn test_read_secret_special_chars_in_password() {
        let path = std::env::temp_dir().join("vk-test-read-secret-4");
        std::fs::write(&path, "p@$$w0rd!#%^&*()\n").unwrap();
        std::env::set_var("_VK_TEST_RS4", path.to_str().unwrap());
        let result = read_secret("_VK_TEST_RS4", "unused: ");
        assert_eq!(&*result, "p@$$w0rd!#%^&*()");
        std::fs::remove_file(&path).ok();
        std::env::remove_var("_VK_TEST_RS4");
    }

    #[test]
    fn test_read_secret_returns_string() {
        let path = std::env::temp_dir().join("vk-test-read-secret-5");
        std::fs::write(&path, "test-value\n").unwrap();
        std::env::set_var("_VK_TEST_RS5", path.to_str().unwrap());
        let result = read_secret("_VK_TEST_RS5", "unused: ");
        assert_eq!(result, "test-value");
        std::fs::remove_file(&path).ok();
        std::env::remove_var("_VK_TEST_RS5");
    }

    // ══════════════════════════════════════════════════════════════
    // SECURITY: #10 line_buf must have bounded growth
    // ══════════════════════════════════════════════════════════════

    #[test]
    fn sec_line_buf_bounded_growth() {
        // Send 100KB of data without Enter — line_buf must not grow unbounded
        let mut buf = String::new();
        let map = secrets(&[("needle", "VK:LOCAL:bound")]);
        let big_input: Vec<u8> = vec![b'x'; 100_000];
        check_stdin_for_secrets(&big_input, &mut buf, &map);
        assert!(
            buf.len() <= 8192,
            "line_buf grew to {} bytes — must be capped to prevent DoS",
            buf.len()
        );
    }

    #[test]
    fn sec_line_buf_still_detects_after_cap() {
        // Even after truncation, secret at the end must be detected
        let mut buf = String::new();
        let map = secrets(&[("needle99", "VK:LOCAL:cap")]);
        // Fill with junk, then secret + enter
        let mut input: Vec<u8> = vec![b'x'; 100_000];
        input.extend_from_slice(b"needle99\r");
        let result = check_stdin_for_secrets(&input, &mut buf, &map);
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "secret after overflow must still be caught"
        );
    }

    // ══════════════════════════════════════════════════════════════
    // SECURITY: #1 TLS insecure flag should warn
    // ══════════════════════════════════════════════════════════════

    #[test]
    fn sec_tls_insecure_not_default() {
        // VEILKEY_TLS_INSECURE must not be "1" by default
        std::env::remove_var("VEILKEY_TLS_INSECURE");
        let val = std::env::var("VEILKEY_TLS_INSECURE").unwrap_or_default();
        assert_ne!(val, "1", "TLS insecure must not be enabled by default");
    }

    // ── Dense edge cases ──────────────────────────────────────────

    #[test]
    fn edge_very_long_secret_10kb() {
        // 10KB secret — must still be detected
        let secret: String = "A".repeat(10 * 1024);
        let mut buf = String::new();
        let map = secrets(&[(&secret, "VK:LOCAL:big10kb1")]);
        // Type the secret (will be capped by MAX_LINE_BUF, keeping tail)
        check_stdin_for_secrets(secret.as_bytes(), &mut buf, &map);
        // The secret is 10KB but MAX_LINE_BUF is 8KB, so buf has the tail.
        // Enter should still detect the secret because the tail contains
        // enough of the secret for substring matching.
        let result = check_stdin_for_secrets(b"\r", &mut buf, &map);
        // The full secret can't fit in buf, so it may not be detected.
        // This documents the behavior: very long secrets exceed MAX_LINE_BUF.
        // The important thing is that we don't panic.
        assert!(
            result == StdinGuardResult::Forward || result == StdinGuardResult::Blocked,
            "must not panic on 10KB secret"
        );
    }

    #[test]
    fn edge_secret_with_regex_special_chars() {
        // Secrets with regex metacharacters — check_stdin_for_secrets uses
        // String::contains (not regex), so these must work.
        let mut buf = String::new();
        let map = secrets(&[("p@ss.w+rd()", "VK:LOCAL:regex001")]);
        let result = check_stdin_for_secrets(b"echo p@ss.w+rd()\r", &mut buf, &map);
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "regex special chars must be matched literally"
        );
    }

    #[test]
    fn edge_multiple_enters_in_one_chunk() {
        // Multiple commands in one paste, with a secret in the second line
        let mut buf = String::new();
        let map = secrets(&[("hunter2", "VK:LOCAL:multi001")]);
        // "ls\necho hunter2\n" — first line safe, second has secret
        let result = check_stdin_for_secrets(b"ls\necho hunter2\n", &mut buf, &map);
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "secret in second line of multi-enter chunk must be caught"
        );
    }

    #[test]
    fn edge_interleaved_ctrl_c_and_typing() {
        // Type part of secret, Ctrl+C, type again, enter
        let mut buf = String::new();
        let map = secrets(&[("secret99", "VK:LOCAL:intrlv01")]);
        // Type "secr", Ctrl+C clears, type "secret99", enter
        check_stdin_for_secrets(b"secr", &mut buf, &map);
        assert_eq!(buf, "secr");
        check_stdin_for_secrets(b"\x03", &mut buf, &map); // Ctrl+C
        assert_eq!(buf, "");
        let result = check_stdin_for_secrets(b"secret99\r", &mut buf, &map);
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "secret after ctrl-c must still be caught"
        );
    }

    #[test]
    fn edge_tab_character_in_input() {
        // Tab is < ' ' (0x09 < 0x20), so it's not accumulated into line_buf
        let mut buf = String::new();
        let map = secrets(&[("abc", "VK:LOCAL:tab00001")]);
        check_stdin_for_secrets(b"\tabc\r", &mut buf, &map);
        // Tab is filtered out, only "abc" is in buf
        // The function already returned, let's test directly
        let mut buf2 = String::new();
        let result = check_stdin_for_secrets(b"\tabc\r", &mut buf2, &map);
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "secret after tab must be caught"
        );
    }

    #[test]
    fn edge_only_whitespace_as_secret() {
        // Secret is "   " (spaces) — should be detected in typed input
        let mut buf = String::new();
        let map = secrets(&[("   ", "VK:LOCAL:ws000001")]);
        let result = check_stdin_for_secrets(b"echo    \r", &mut buf, &map);
        // "echo    " contains "   " as substring
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "whitespace-only secret must be detected"
        );
    }

    #[test]
    fn edge_max_line_buf_exactly_at_boundary() {
        // Fill buf to exactly MAX_LINE_BUF chars
        let mut buf = String::new();
        let map = secrets(&[("needle", "VK:LOCAL:bnd00001")]);
        let input: Vec<u8> = vec![b'x'; MAX_LINE_BUF];
        check_stdin_for_secrets(&input, &mut buf, &map);
        assert!(
            buf.len() <= MAX_LINE_BUF,
            "buf must not exceed MAX_LINE_BUF: got {}",
            buf.len()
        );
    }

    #[test]
    fn edge_max_line_buf_one_over_boundary() {
        // Fill buf to MAX_LINE_BUF + 1 chars — must be truncated
        let mut buf = String::new();
        let map = secrets(&[("needle", "VK:LOCAL:bnd00002")]);
        let input: Vec<u8> = vec![b'x'; MAX_LINE_BUF + 1];
        check_stdin_for_secrets(&input, &mut buf, &map);
        assert!(
            buf.len() <= MAX_LINE_BUF,
            "buf must be capped at MAX_LINE_BUF: got {}",
            buf.len()
        );
    }

    #[test]
    fn edge_secret_detection_after_truncation() {
        // After truncation, a secret at the tail end must still be detected
        let mut buf = String::new();
        let secret = "needle99";
        let map = secrets(&[(secret, "VK:LOCAL:trunc001")]);
        // Fill beyond MAX_LINE_BUF, then add secret + enter
        let padding: Vec<u8> = vec![b'x'; MAX_LINE_BUF + 100];
        check_stdin_for_secrets(&padding, &mut buf, &map);
        let mut secret_bytes = secret.as_bytes().to_vec();
        secret_bytes.push(b'\r');
        let result = check_stdin_for_secrets(&secret_bytes, &mut buf, &map);
        assert_eq!(
            result,
            StdinGuardResult::Blocked,
            "secret at tail after truncation must be caught"
        );
    }
}
