use std::collections::HashSet;
use std::io::{self, Read, Write};
use std::net::{TcpListener, TcpStream};
use std::thread;
use std::time::Duration;

fn proxy_max_header() -> usize {
    std::env::var("VEILKEY_PROXY_MAX_HEADER")
        .ok()
        .and_then(|v| v.parse::<usize>().ok())
        .unwrap_or(16384)
}

fn proxy_timeout() -> Duration {
    let secs = std::env::var("VEILKEY_PROXY_TIMEOUT")
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(30);
    Duration::from_secs(secs)
}

fn default_https_port() -> String {
    std::env::var("VEILKEY_PROXY_DEFAULT_HTTPS_PORT").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_PROXY_DEFAULT_HTTPS_PORT is required");
        std::process::exit(1);
    })
}

fn default_http_port() -> String {
    std::env::var("VEILKEY_PROXY_DEFAULT_HTTP_PORT").unwrap_or_else(|_| {
        eprintln!("error: VEILKEY_PROXY_DEFAULT_HTTP_PORT is required");
        std::process::exit(1);
    })
}

pub fn run(listen: &str, allow_hosts: Vec<String>) {
    let listener = match TcpListener::bind(listen) {
        Ok(l) => l,
        Err(e) => {
            eprintln!("proxy listen failed: {}", e);
            std::process::exit(1);
        }
    };
    let allow_set: HashSet<String> =
        allow_hosts.into_iter().map(|h| h.to_lowercase()).collect();

    for stream in listener.incoming() {
        match stream {
            Ok(s) => {
                let allow = allow_set.clone();
                thread::spawn(move || {
                    if let Err(e) = handle_connection(s, &allow) {
                        eprintln!("proxy error: {}", e);
                    }
                });
            }
            Err(e) => eprintln!("accept error: {}", e),
        }
    }
}

fn host_allowed(host: &str, allow_set: &HashSet<String>) -> bool {
    allow_set.is_empty() || allow_set.contains(&host.to_lowercase())
}

fn handle_connection(mut stream: TcpStream, allow_set: &HashSet<String>) -> io::Result<()> {
    stream.set_read_timeout(Some(proxy_timeout()))?;

    // Read HTTP request headers
    let mut buf = Vec::new();
    let mut tmp = [0u8; 1];
    loop {
        let n = stream.read(&mut tmp)?;
        if n == 0 {
            return Ok(());
        }
        buf.push(tmp[0]);
        if buf.ends_with(b"\r\n\r\n") {
            break;
        }
        if buf.len() > proxy_max_header() {
            return Ok(());
        }
    }

    let header_str = String::from_utf8_lossy(&buf).to_string();
    let mut lines = header_str.lines();
    let request_line = match lines.next() {
        Some(l) => l,
        None => return Ok(()),
    };
    let parts: Vec<&str> = request_line.splitn(3, ' ').collect();
    if parts.len() < 3 {
        return Ok(());
    }
    let method = parts[0];
    let target = parts[1];

    if method == "CONNECT" {
        // HTTPS CONNECT tunnel
        let host = target.split(':').next().unwrap_or(target);
        if !host_allowed(host, allow_set) {
            let _ = stream.write_all(b"HTTP/1.1 403 Forbidden\r\n\r\nhost not allowed");
            return Ok(());
        }
        let addr = if target.contains(':') {
            target.to_string()
        } else {
            format!("{}:{}", target, default_https_port())
        };
        match TcpStream::connect(&addr) {
            Err(_) => {
                let _ = stream.write_all(b"HTTP/1.1 502 Bad Gateway\r\n\r\n");
            }
            Ok(upstream) => {
                stream.write_all(b"HTTP/1.1 200 Connection Established\r\n\r\n")?;
                let mut client_r = stream.try_clone()?;
                let mut upstream_w = upstream.try_clone()?;
                thread::spawn(move || {
                    let _ = io::copy(&mut client_r, &mut upstream_w);
                });
                let mut upstream_r = upstream;
                let _ = io::copy(&mut upstream_r, &mut stream);
            }
        }
    } else {
        // Plain HTTP proxy
        let host = if target.starts_with("http") {
            target
                .split("//")
                .nth(1)
                .and_then(|s| s.split('/').next())
                .and_then(|s| s.split(':').next())
                .unwrap_or("")
                .to_string()
        } else {
            header_str
                .lines()
                .find(|l| l.to_lowercase().starts_with("host:"))
                .and_then(|l| l.split_once(':').map(|(_, v)| v))
                .map(|s| s.trim().split(':').next().unwrap_or("").to_string())
                .unwrap_or_default()
        };

        if !host_allowed(&host, allow_set) {
            let _ = stream.write_all(b"HTTP/1.1 403 Forbidden\r\n\r\nhost not allowed");
            return Ok(());
        }

        let addr = if target.starts_with("http") {
            let hp = target
                .split("//")
                .nth(1)
                .and_then(|s| s.split('/').next())
                .unwrap_or("");
            if hp.contains(':') {
                hp.to_string()
            } else {
                format!("{}:{}", hp, default_http_port())
            }
        } else {
            format!("{}:{}", host, default_http_port())
        };

        match TcpStream::connect(&addr) {
            Err(_) => {
                let _ = stream.write_all(b"HTTP/1.1 502 Bad Gateway\r\n\r\n");
            }
            Ok(mut upstream) => {
                upstream.write_all(&buf)?;
                let mut client_r = stream.try_clone()?;
                let mut upstream_w = upstream.try_clone()?;
                thread::spawn(move || {
                    let _ = io::copy(&mut client_r, &mut upstream_w);
                });
                let _ = io::copy(&mut upstream, &mut stream);
            }
        }
    }
    Ok(())
}
