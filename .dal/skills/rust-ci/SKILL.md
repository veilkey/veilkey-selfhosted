# Rust CI — Rust Continuous Integration Pipeline

## Purpose

Manages the Rust CI pipeline for veil-cli, the terminal wrapper component.

## Commands

```bash
# Build
cargo build
cargo build --release

# Test
cargo test

# Lint
cargo clippy -- -D warnings

# Format check
cargo fmt -- --check

# Security audit (if cargo-audit is installed)
cargo audit
```

## CI Checks

1. **Build** — `cargo build` must succeed with zero errors
2. **Test** — `cargo test` all tests pass
3. **Clippy** — `cargo clippy -- -D warnings` no warnings
4. **Format** — `cargo fmt -- --check` code is properly formatted
5. **Audit** — `cargo audit` no known vulnerabilities in dependencies

## veil-cli Specifics

- PTY handling code must be tested on both macOS and Linux
- Secret pattern regex compilation must not panic
- Memory safety is critical — secrets must be wiped after use
- Cross-compilation targets: x86_64-apple-darwin, x86_64-unknown-linux-gnu, aarch64-apple-darwin

## Failure Handling

- On test failure: report exact test name, file, and line
- On clippy warning: report the lint name and suggested fix
- On audit finding: report CVE, affected crate, and minimum safe version
