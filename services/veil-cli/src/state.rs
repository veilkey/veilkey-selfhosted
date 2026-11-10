use std::env;
use std::fs::{self, DirBuilder, File, OpenOptions};
use std::io::{self, Write};
use std::os::unix::fs::{DirBuilderExt, MetadataExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};

pub fn default_state_dir() -> PathBuf {
    if let Some(path) = env::var_os("XDG_STATE_HOME").filter(|p| !p.is_empty()) {
        if Path::new(&path).is_absolute() {
            return PathBuf::from(path).join("veilkey-cli");
        }
    }
    let home = env::var_os("HOME")
        .filter(|p| !p.is_empty())
        .expect("HOME or VEILKEY_STATE_DIR is required for private CLI state");
    PathBuf::from(home).join(".local/state/veilkey-cli")
}

/// Create private state without accepting a symlink or another user's directory.
pub fn ensure_private_dir(path: &Path) -> io::Result<()> {
    DirBuilder::new().recursive(true).mode(0o700).create(path)?;
    let metadata = fs::symlink_metadata(path)?;
    if !metadata.is_dir() || metadata.uid() != unsafe { libc::geteuid() } {
        return Err(io::Error::new(
            io::ErrorKind::PermissionDenied,
            "state directory must be owned by the current user and not be a symlink",
        ));
    }
    fs::set_permissions(path, fs::Permissions::from_mode(0o700))
}

pub fn state_dir() -> PathBuf {
    let path = match env::var_os("VEILKEY_STATE_DIR").filter(|p| !p.is_empty()) {
        Some(path) => PathBuf::from(path),
        None => default_state_dir(),
    };
    ensure_private_dir(&path).unwrap_or_else(|error| {
        eprintln!("veilkey: cannot initialize private state: {}", error);
        std::process::exit(1);
    });
    path
}

pub fn open_private_file(path: &Path, append: bool) -> io::Result<File> {
    let file = OpenOptions::new()
        .write(true)
        .create(true)
        .append(append)
        .mode(0o600)
        .custom_flags(libc::O_NOFOLLOW | libc::O_NONBLOCK)
        .open(path)?;
    let metadata = file.metadata()?;
    if !metadata.is_file() || metadata.uid() != unsafe { libc::geteuid() } || metadata.nlink() != 1
    {
        return Err(io::Error::new(
            io::ErrorKind::PermissionDenied,
            "state file must be a private regular file",
        ));
    }
    file.set_permissions(fs::Permissions::from_mode(0o600))?;
    Ok(file)
}

pub fn write_private_file(path: &Path, contents: &[u8]) -> io::Result<()> {
    let mut file = open_private_file(path, false)?;
    file.set_len(0)?;
    file.write_all(contents)
}

pub fn paste_mode_path() -> PathBuf {
    state_dir().join("paste-mode")
}

pub fn current_paste_mode() -> &'static str {
    match fs::read_to_string(paste_mode_path()) {
        Ok(data) if data.trim().eq_ignore_ascii_case("off") => "off",
        _ => "on",
    }
}

pub fn set_paste_mode(mode: &str) -> Result<(), String> {
    let mode = mode.trim().to_lowercase();
    if mode != "on" && mode != "off" {
        return Err(format!("invalid paste mode: {}", mode));
    }
    write_private_file(&paste_mode_path(), format!("{}\n", mode).as_bytes())
        .map_err(|error| error.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::symlink;

    #[test]
    fn state_directory_and_files_are_owner_only() {
        let root = tempfile::tempdir().unwrap();
        let dir = root.path().join("state");
        fs::create_dir(&dir).unwrap();
        fs::set_permissions(&dir, fs::Permissions::from_mode(0o777)).unwrap();
        ensure_private_dir(&dir).unwrap();
        let path = dir.join("mode");
        write_private_file(&path, b"off\n").unwrap();
        assert_eq!(fs::metadata(dir).unwrap().mode() & 0o777, 0o700);
        assert_eq!(fs::metadata(path).unwrap().mode() & 0o777, 0o600);
    }

    #[test]
    fn state_rejects_symlink_directories_and_files() {
        let root = tempfile::tempdir().unwrap();
        let target = root.path().join("target");
        fs::create_dir(&target).unwrap();
        let link = root.path().join("link");
        symlink(&target, &link).unwrap();
        assert!(ensure_private_dir(&link).is_err());
        let target_file = target.join("file");
        fs::write(&target_file, b"unchanged").unwrap();
        let file_link = root.path().join("file-link");
        symlink(&target_file, &file_link).unwrap();
        assert!(write_private_file(&file_link, b"new").is_err());
        assert_eq!(fs::read(target_file).unwrap(), b"unchanged");
    }
}
