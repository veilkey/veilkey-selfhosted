use nix::libc;
use nix::sys::signal::{self, SigHandler, Signal};
use nix::unistd::{execvp, fork, ForkResult};
use std::ffi::CString;
use std::io;
use std::os::fd::{AsRawFd, RawFd};
use std::sync::{Arc, Mutex, RwLock};
use std::time::Duration;

use crate::api::VeilKeyClient;
use crate::config::{load_config, CompiledPattern};
use crate::state::state_dir;

use super::masker;
use super::sync as mask_sync;

static mut MASTER_FD: RawFd = -1;

extern "C" fn handle_sigwinch(_: libc::c_int) {
    unsafe {
        if MASTER_FD >= 0 {
            let mut ws: libc::winsize = std::mem::zeroed();
            libc::ioctl(0, libc::TIOCGWINSZ, &mut ws);
            libc::ioctl(MASTER_FD, libc::TIOCSWINSZ, &ws);
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
    eprintln!("[veilkey] loaded {} secret(s) from vaults", mask_map.len());

    // 2. Resolve VK refs in environment variables
    let vk_re = regex::Regex::new(crate::detector::VEILKEY_RE_STR).unwrap();
    let mut child_env: Vec<(String, String)> = Vec::new();
    for (key, value) in std::env::vars() {
        if vk_re.is_match(&value) {
            let resolved = vk_re
                .replace_all(&value, |caps: &regex::Captures| match client.resolve(&caps[0]) {
                    Ok(v) => {
                        if !mask_map.iter().any(|(p, _)| p == &v) {
                            mask_map.push((v.clone(), caps[0].to_string()));
                        }
                        v
                    }
                    Err(_) => caps[0].to_string(),
                })
                .to_string();
            child_env.push((key, resolved));
        }
    }
    mask_map.sort_by(|a, b| b.0.len().cmp(&a.0.len()));

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
        MASTER_FD = master_fd;
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
                unsafe {
                    let mut raw = old_termios;
                    libc::cfmakeraw(&mut raw);
                    libc::tcsetattr(stdin_fd, libc::TCSANOW, &raw);
                }
            }

            let mask_map = Arc::new(RwLock::new(mask_map));
            let patterns = Arc::new(patterns);
            let client = Arc::new(client);

            // Background mask_map sync
            mask_sync::spawn_mask_map_sync(mask_map.clone(), client.clone());

            // Recent input tracking
            let recent_input = Arc::new(Mutex::new(String::new()));

            // stdin → PTY master (input thread)
            let master_wr = master_fd;
            let input_tracker = recent_input.clone();
            std::thread::spawn(move || {
                let mut buf = [0u8; 4096];
                loop {
                    let n = unsafe { libc::read(stdin_fd, buf.as_mut_ptr() as _, buf.len()) };
                    if n <= 0 {
                        break;
                    }
                    if let Ok(s) = std::str::from_utf8(&buf[..n as usize]) {
                        let mut tracker = input_tracker.lock().unwrap();
                        tracker.push_str(s);
                        if tracker.len() > 4096 {
                            let start = tracker.len() - 4096;
                            *tracker = tracker[start..].to_string();
                        }
                    }
                    unsafe {
                        libc::write(master_wr, buf.as_ptr() as _, n as _);
                    }
                }
            });

            // PTY master → stdout (output thread with masking)
            let mask = mask_map.clone();
            let input_ref = recent_input.clone();
            let stdout_fd = io::stdout().as_raw_fd();
            let mut partial_buf: Vec<u8> = Vec::new();

            let mut buf = [0u8; 32768];
            loop {
                let n = unsafe { libc::read(master_fd, buf.as_mut_ptr() as _, buf.len()) };
                if n <= 0 {
                    break;
                }
                let n = n as usize;
                let chunk = &buf[..n];

                if let Some(last_nl) = chunk.iter().rposition(|&b| b == b'\n') {
                    // Newline found — flush partial + chunk through masker
                    let mut to_mask = Vec::new();
                    to_mask.extend_from_slice(&partial_buf);
                    to_mask.extend_from_slice(&chunk[..last_nl + 1]);
                    partial_buf.clear();

                    let ri = input_ref.lock().unwrap().clone();
                    let masked =
                        masker::mask_output(&to_mask, &mask.read().unwrap(), &patterns, &client, &ri);
                    unsafe {
                        libc::write(stdout_fd, masked.as_ptr() as _, masked.len());
                    }

                    // Remainder → buffer
                    if last_nl + 1 < n {
                        partial_buf.extend_from_slice(&chunk[last_nl + 1..]);
                    }
                } else {
                    // No newline — buffer, flush after short wait if no more data
                    partial_buf.extend_from_slice(chunk);
                    std::thread::sleep(Duration::from_millis(20));
                    let mut peek = [0u8; 1];
                    unsafe {
                        let flags = libc::fcntl(master_fd, libc::F_GETFL);
                        libc::fcntl(master_fd, libc::F_SETFL, flags | libc::O_NONBLOCK);
                        let peek_result = libc::read(master_fd, peek.as_mut_ptr() as _, 1);
                        libc::fcntl(master_fd, libc::F_SETFL, flags);
                        if peek_result > 0 {
                            partial_buf.push(peek[0]);
                        } else {
                            // No more data — flush as-is (prompt, echo-back)
                            libc::write(
                                stdout_fd,
                                partial_buf.as_ptr() as _,
                                partial_buf.len(),
                            );
                            partial_buf.clear();
                        }
                    }
                }
            }

            // Flush remaining
            if !partial_buf.is_empty() {
                let ri = input_ref.lock().unwrap().clone();
                let masked = masker::mask_output(
                    &partial_buf,
                    &mask.read().unwrap(),
                    &patterns,
                    &client,
                    &ri,
                );
                unsafe {
                    libc::write(stdout_fd, masked.as_ptr() as _, masked.len());
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
