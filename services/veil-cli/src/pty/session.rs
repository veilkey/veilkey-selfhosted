use nix::libc;
use nix::sys::signal::{self, SigHandler, Signal};
use nix::unistd::{execvp, fork, ForkResult};
use std::ffi::CString;
use std::io;
use std::os::fd::{AsRawFd, RawFd};
use std::sync::atomic::{AtomicBool, AtomicI32, Ordering};
use std::sync::{Arc, Mutex, RwLock};

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

pub fn run(args: &[String], api_url: &str, _log_path: &str, patterns_file: Option<&str>) {
    let shell_args: Vec<String> = if args.is_empty() {
        let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/bash".to_string());
        vec![shell]
    } else {
        args.to_vec()
    };

    let client = VeilKeyClient::new(api_url);

    // Authenticate with admin password before fetching secrets
    let password = rpassword::prompt_password("VeilKey password: ").unwrap_or_default();
    if password.is_empty() {
        eprintln!("[veilkey] password is required");
        std::process::exit(1);
    }
    if let Err(e) = client.admin_login(&password) {
        eprintln!("[veilkey] authentication failed: {}", e);
        std::process::exit(1);
    }

    // Load detection patterns
    let cfg = load_config(patterns_file).ok();
    let patterns: Vec<CompiledPattern> = cfg.map(|c| c.patterns).unwrap_or_default();

    // Save PID file
    let sd = state_dir();
    let _ = std::fs::create_dir_all(&sd);
    let pid_path = sd.join("guard.pid");
    let _ = std::fs::write(&pid_path, format!("{}", std::process::id()));

    // 1. Fetch mask_map (fail-closed)
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
    let vk_re = regex::Regex::new(crate::detector::VEILKEY_RE_STR).unwrap();
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
            let patterns = Arc::new(patterns);
            let client = Arc::new(client);

            // Background mask_map sync
            mask_sync::spawn_mask_map_sync(mask_map.clone(), client.clone());

            // Recent input tracking
            let recent_input = Arc::new(Mutex::new(String::new()));

            // stdin → PTY master (input thread)
            // Terminal response sequences (DSR, OSC) are passed through to PTY
            // but excluded from recent_input to avoid false masking skips.
            let master_wr = master_fd;
            let input_tracker = recent_input.clone();
            std::thread::spawn(move || {
                let mut buf = [0u8; 4096];
                loop {
                    let n = unsafe { read_eintr(stdin_fd, &mut buf) };
                    if n <= 0 {
                        break;
                    }
                    let data = &buf[..n as usize];
                    // Track user input (exclude terminal response sequences)
                    if let Ok(s) = std::str::from_utf8(data) {
                        // Filter out escape sequences from input tracking
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
                    // Always forward raw data to PTY (including escape sequences)
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
                let chunk = &buf[..n as usize];

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
                    &patterns,
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
