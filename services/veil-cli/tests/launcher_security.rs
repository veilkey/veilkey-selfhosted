#![cfg(unix)]

use std::fs;
use std::os::unix::fs::{symlink, PermissionsExt};
use std::path::Path;
use std::process::Command;

fn script(root: &Path, content: &str) -> std::path::PathBuf {
    let path = root.join("cli");
    fs::write(&path, content).unwrap();
    fs::set_permissions(&path, fs::Permissions::from_mode(0o700)).unwrap();
    path
}

fn launcher(root: &Path, cli: &Path) -> Command {
    let home = root.join("home");
    fs::create_dir_all(&home).unwrap();
    let mut command = Command::new(env!("CARGO_BIN_EXE_veil"));
    command
        .env_clear()
        .env("HOME", home)
        .env("PATH", "/usr/bin:/bin")
        .env("VEILKEY_CLI_BIN", cli)
        .current_dir(root);
    command
}

#[test]
fn project_and_parent_env_cannot_override_operator_security_settings() {
    let root = tempfile::tempdir().unwrap();
    let repo = root.path().join("project");
    let nested = repo.join("nested");
    fs::create_dir_all(repo.join(".veilkey")).unwrap();
    fs::create_dir(&nested).unwrap();
    fs::write(repo.join(".veilkey/env"), "VEILKEY_CLI_BIN=/nonexistent-project-command\nVEILKEY_LOCALVAULT_URL=https://project.invalid\nVEILKEY_VAULTCENTER_URL=https://project.invalid\nVEILKEY_TLS_INSECURE=1\nVEILKEY_PASSWORD_FILE=/project/password\nPATH=/project/bin\n").unwrap();
    let cli = script(root.path(), "#!/bin/sh\nprintf '%s\\n' \"$VEILKEY_API\" \"$VEILKEY_VAULTCENTER_URL\" \"$VEILKEY_TLS_INSECURE\" \"$VEILKEY_PASSWORD_FILE\" \"$PATH\"\n");
    for cwd in [&repo, &nested] {
        let output = launcher(root.path(), &cli)
            .current_dir(cwd)
            .env("VEILKEY_LOCALVAULT_URL", "https://operator.invalid")
            .env("VEILKEY_VAULTCENTER_URL", "https://center.invalid")
            .env("VEILKEY_TLS_INSECURE", "0")
            .arg("status")
            .output()
            .unwrap();
        assert!(output.status.success());
        assert_eq!(
            String::from_utf8(output.stdout).unwrap(),
            "https://operator.invalid\nhttps://center.invalid\n0\n\n/usr/bin:/bin\n"
        );
    }
}

#[test]
fn operator_home_and_explicit_environment_files_remain_supported() {
    let root = tempfile::tempdir().unwrap();
    let cli = script(root.path(), "#!/bin/sh\nprintf '%s\\n' \"$VEILKEY_API\"\n");
    let mut command = launcher(root.path(), &cli);
    fs::create_dir_all(root.path().join("home/.veilkey")).unwrap();
    fs::write(
        root.path().join("home/.veilkey/env"),
        "VEILKEY_LOCALVAULT_URL=https://home.invalid\n",
    )
    .unwrap();
    let output = command.arg("status").output().unwrap();
    assert!(output.status.success());
    assert_eq!(output.stdout, b"https://home.invalid\n");
    let explicit = root.path().join("operator.env");
    fs::write(
        &explicit,
        "export VEILKEY_LOCALVAULT_URL=https://explicit.invalid\n",
    )
    .unwrap();
    let output = launcher(root.path(), &cli)
        .env("VEILKEY_ENV", explicit)
        .arg("status")
        .output()
        .unwrap();
    assert!(output.status.success());
    assert_eq!(output.stdout, b"https://explicit.invalid\n");
}

#[test]
fn shell_rcfiles_are_unique_private_and_removed_after_exit() {
    let root = tempfile::tempdir().unwrap();
    let target = root.path().join("unchanged");
    fs::write(&target, "unchanged").unwrap();
    symlink(&target, root.path().join("veil-bashrc")).unwrap();
    let cli = script(
        root.path(),
        "#!/bin/sh\nfor last; do :; done\nprintf '%s\\n' \"$last\"\nstat -c '%a' \"$last\"\n",
    );
    let mut paths = Vec::new();
    for _ in 0..2 {
        let output = launcher(root.path(), &cli)
            .env("TMPDIR", root.path())
            .output()
            .unwrap();
        assert!(output.status.success());
        let text = String::from_utf8(output.stdout).unwrap();
        let lines: Vec<_> = text.lines().collect();
        assert_eq!(lines[1], "600");
        assert!(!Path::new(lines[0]).exists());
        paths.push(lines[0].to_string());
    }
    assert_ne!(paths[0], paths[1]);
    assert_eq!(fs::read_to_string(target).unwrap(), "unchanged");
}

#[test]
fn rcfile_creation_failure_does_not_start_shell() {
    let root = tempfile::tempdir().unwrap();
    let cli = script(root.path(), "#!/bin/sh\nprintf 'started\\n'\n");
    let output = launcher(root.path(), &cli)
        .env("TMPDIR", root.path().join("missing"))
        .output()
        .unwrap();
    assert!(!output.status.success());
    assert!(output.stdout.is_empty());
}

#[test]
fn default_cli_state_uses_private_user_storage_not_tmpdir() {
    let root = tempfile::tempdir().unwrap();
    let home = root.path().join("home");
    let tmp = root.path().join("shared-tmp");
    fs::create_dir_all(&home).unwrap();
    fs::create_dir(&tmp).unwrap();
    let output = Command::new(env!("CARGO_BIN_EXE_veilkey-cli"))
        .env_clear()
        .env("HOME", &home)
        .env("TMPDIR", &tmp)
        .arg("paste-mode")
        .arg("off")
        .output()
        .unwrap();
    assert!(output.status.success());
    let state = home.join(".local/state/veilkey-cli");
    assert_eq!(
        fs::metadata(&state).unwrap().permissions().mode() & 0o777,
        0o700
    );
    assert_eq!(
        fs::metadata(state.join("paste-mode"))
            .unwrap()
            .permissions()
            .mode()
            & 0o777,
        0o600
    );
    assert!(!tmp.join("veilkey-cli").exists());
}

#[test]
fn localvault_requires_a_local_release_installer() {
    let root = tempfile::tempdir().unwrap();
    let cli = script(root.path(), "#!/bin/sh\nexit 0\n");
    // Keep this regression offline even when it is run against an older launcher.
    let bin = root.path().join("bin");
    fs::create_dir(&bin).unwrap();
    let curl = bin.join("curl");
    fs::write(&curl, "#!/bin/sh\nexit 1\n").unwrap();
    fs::set_permissions(&curl, fs::Permissions::from_mode(0o700)).unwrap();
    let output = launcher(root.path(), &cli)
        .env("PATH", format!("{}:/usr/bin:/bin", bin.display()))
        .env("VEILKEY_VAULTCENTER_URL", "https://operator.invalid")
        .args(["localvault", "init"])
        .output()
        .unwrap();
    assert!(!output.status.success());
    assert!(String::from_utf8(output.stderr)
        .unwrap()
        .contains("latest release tag"));
}

#[test]
fn invalid_selected_ca_never_falls_back_to_insecure_tls() {
    let root = tempfile::tempdir().unwrap();
    let ca = root.path().join("ca.pem");
    for contents in [None, Some(""), Some("not a certificate")] {
        if let Some(contents) = contents {
            fs::write(&ca, contents).unwrap();
        }
        let output = Command::new(env!("CARGO_BIN_EXE_veilkey-cli"))
            .env_clear()
            .env("HOME", root.path())
            .env("VEILKEY_LOCALVAULT_URL", "https://localhost:0")
            .env("VEILKEY_CA_CERT", &ca)
            .env("VEILKEY_TLS_INSECURE", "1")
            .arg("status")
            .output()
            .unwrap();
        assert!(!output.status.success());
        assert!(String::from_utf8(output.stderr).unwrap().contains("CA"));
    }
}
