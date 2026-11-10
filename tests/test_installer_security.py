"""Offline regression tests for release installation and secure installer output.

Run with python3 -B -m unittest discover -s tests -p test_installer_security.py -v.
No services, builds, package installs, or network access are used.
"""

import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(os.environ.get("VEILKEY_TEST_ROOT", Path(__file__).resolve().parents[1]))
SCRATCH = Path.home() / "tmp" / "veilkey-installer-tests"


class InstallerSecurityTests(unittest.TestCase):
    def setUp(self):
        SCRATCH.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(dir=SCRATCH)
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name)
        self.bin = self.work / "bin"
        self.bin.mkdir()
        self.home = self.work / "home"
        self.home.mkdir()
        self.env = os.environ.copy()
        for key in tuple(self.env):
            if key.startswith("VEILKEY_") or key in ("SSL_CERT_FILE", "SSL_CERT_DIR"):
                self.env.pop(key)
        self.env.update(HOME=str(self.home), PATH=f"{self.bin}:{os.environ['PATH']}")
        self.stub("getent", "printf '192.0.2.42 STREAM vc.example\\n192.0.2.42 DGRAM vc.example\\n'")
        self.stub("curl", '''
case " $* " in
    *" -o "*)
        while [ "$1" != "-o" ]; do shift; done
        printf '#!/bin/bash\\nexit 0\\n' > "$2"
        ;;
    *) printf '{"status":"ok"}';;
esac
''')
        if sys.platform == "darwin":
            # Proxmox uses GNU sed; adapt only its in-place argument for macOS.
            self.stub("sed", '''
if [ "$1" = "-i" ]; then
    shift
    exec /usr/bin/sed -i '' "$@"
fi
exec /usr/bin/sed "$@"
''')

    def stub(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/bash\nset -eu\n" + body + "\n")
        path.chmod(0o755)

    def run_shell(self, args, cwd=None, env=None):
        result = subprocess.run(
            ["bash", *args], cwd=cwd or ROOT, env=env or self.env,
            text=True, capture_output=True, timeout=20,
        )
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        return result

    def test_no_registry_install_or_badges(self):
        files = [ROOT / "README.md", ROOT / "README.ko.md", *ROOT.glob("install/**/*.md"), *ROOT.glob("docs/**/*.md")]
        for path in files:
            if path.name == "security-model.md":
                continue
            with self.subTest(path=str(path.relative_to(ROOT))):
                text = path.read_text()
                self.assertNotIn("npmjs.com/package/", text)
                self.assertNotIn("img.shields.io/npm/", text)
                self.assertNotRegex(text, r"npm\s+(?:install|i|update|upgrade)\s+(?:--global|-g)\s+veilkey(?:-cli)?\b")
        installer = (ROOT / "install/macos/veil-cli/install.sh").read_text()
        self.assertIn('npm install -g "$REPO_ROOT/packages/veil-cli"', installer)

    def test_readmes_clone_latest_release_tag(self):
        for name in ("README.md", "README.ko.md"):
            text = (ROOT / name).read_text()
            self.assertIn('RELEASE_TAG="<latest-release-tag>"', text)
            self.assertIn('git clone --depth 1 --branch "$RELEASE_TAG"', text)
            self.assertIn("bash install/macos/bootstrap/install-all.sh", text)

    def test_no_external_installer_disables_tls_by_default(self):
        for path in ROOT.glob("install/**/*.sh"):
            text = path.read_text()
            with self.subTest(path=str(path.relative_to(ROOT))):
                self.assertNotRegex(text, r"VEILKEY_TLS_INSECURE(?::-[\"']?)?=1")
                self.assertNotIn('${VEILKEY_TLS_INSECURE:-1}', text)
        compose = (ROOT / "docker-compose.yml").read_text()
        self.assertIn("Single-host Docker-internal TLS exception", compose)

    def test_no_private_range_or_loopback_trusted_defaults(self):
        for name in (".env.example", "services/localvault/.env.example"):
            text = (ROOT / name).read_text()
            self.assertRegex(text, r"(?m)^VEILKEY_TRUSTED_IPS=$")
        for path in [*ROOT.glob("install/**/*"), *ROOT.glob("docs/**/*.md")]:
            if not path.is_file() or path.name == "security-model.md":
                continue
            for line in path.read_text().splitlines():
                if "TRUSTED_IPS" in line:
                    with self.subTest(path=str(path.relative_to(ROOT))):
                        self.assertNotRegex(line, r"10\.0\.0\.0/8|172\.16\.0\.0/12|192\.168\.0\.0/16|127\.0\.0\.1|::1")

    def test_common_cli_writes_verified_tls_and_ca_path(self):
        config = self.work / "config"
        self.env.update(VEILKEY_URL="https://vc.example:10181", VEILKEY_BIN_DIR=str(self.bin),
                        VEILKEY_CONFIG_DIR=str(config), VEILKEY_BINARY_URL="https://unused.invalid/binaries",
                        VEILKEY_CA_CERT="/public/verified-ca.pem")
        self.run_shell([str(ROOT / "install/common/install-veil-cli.sh")])
        text = (config / "env").read_text()
        self.assertIn("export VEILKEY_TLS_INSECURE=0", text)
        self.assertIn('export VEILKEY_CA_CERT="/public/verified-ca.pem"', text)
        self.assertEqual(config.stat().st_mode & 0o777, 0o700)
        self.assertEqual((config / "env").stat().st_mode & 0o777, 0o600)

    def test_macos_cli_keeps_local_package_and_verified_tls(self):
        repo = self.work / "release"
        (repo / "services/veil-cli").mkdir(parents=True)
        (repo / "Cargo.toml").touch()
        prefix = self.work / "prefix"
        self.env.update(VEILKEY_CA_CERT="/public/verified-ca.pem", TEST_PREFIX=str(prefix))
        self.stub("cargo", "exit 0")
        self.stub("npm", 'if [ "$1" = "prefix" ]; then printf "%s\\n" "$TEST_PREFIX"; fi')
        self.run_shell([str(ROOT / "install/macos/veil-cli/install.sh")], cwd=repo)
        config = self.home / ".veilkey"
        self.assertTrue((config / "env").is_file(), "Installer must configure the trusted user environment")
        text = (config / "env").read_text()
        self.assertIn("export VEILKEY_TLS_INSECURE=0", text)
        self.assertIn('export VEILKEY_CA_CERT="/public/verified-ca.pem"', text)
        self.assertEqual(config.stat().st_mode & 0o777, 0o700)
        self.assertEqual((config / "env").stat().st_mode & 0o777, 0o600)
        self.assertFalse((repo / ".veilkey/env").exists())
        custom = self.work / "custom-config"
        self.env["VEILKEY_CONFIG_DIR"] = str(custom)
        result = self.run_shell([str(ROOT / "install/macos/veil-cli/install.sh")], cwd=repo)
        self.assertTrue((custom / "env").is_file())
        self.assertIn(f"VEILKEY_ENV={custom}/env", result.stdout)

    def test_macos_uninstall_preserves_user_settings(self):
        config = self.work / "custom-config"
        config.mkdir()
        env_file = config / "env"
        settings = config / "veilkey.yml"
        settings.write_text("threshold: 0.8\n")
        env_file.write_text("# user-maintained environment\n")
        self.env["VEILKEY_CONFIG_DIR"] = str(config)
        self.stub("npm", "exit 0")
        uninstaller = str(ROOT / "install/macos/veil-cli/uninstall.sh")
        self.run_shell([uninstaller])
        self.assertTrue(env_file.exists())
        env_file.write_text("# Generated by VeilKey CLI installer.\n")
        self.run_shell([uninstaller])
        self.assertFalse(env_file.exists())
        self.assertEqual(settings.read_text(), "threshold: 0.8\n")

    def test_common_localvault_generates_narrow_verified_config(self):
        data = self.work / "data"
        (data / "certs").mkdir(parents=True)
        for name in ("cert.pem", "key.pem"):
            (data / "certs" / name).touch()
        self.env.update(VEILKEY_CENTER_URL="https://vc.example:10181", VEILKEY_REG_TOKEN="test-token",
                        VEILKEY_BIN_DIR=str(self.bin), VEILKEY_DATA_DIR=str(data),
                        VEILKEY_BINARY_URL="https://unused.invalid/binary", VEILKEY_SYSTEMD="0",
                        SSL_CERT_FILE="/public/verified-ca.pem")
        self.run_shell([str(ROOT / "install/common/install-localvault.sh")])
        text = (data / ".env").read_text()
        self.assertIn("VEILKEY_TRUSTED_IPS=192.0.2.42\n", text)
        self.assertIn("VEILKEY_TLS_INSECURE=0\n", text)
        self.assertIn("SSL_CERT_FILE=/public/verified-ca.pem\n", text)
        self.assertNotIn("PASSWORD=", text)
        script = (ROOT / "install/common/install-localvault.sh").read_text()
        self.assertIn('--cacert "$TLS_CERT"', script)

    def proxmox_config(self, initial=None):
        script = (ROOT / "install/proxmox-lxc-debian/install-localvault.sh").read_text()
        section = script.split("# --- [6/10]", 1)[1].split("# --- [7/10]", 1)[0]
        data = self.work / "data"
        env_file = self.work / ".env"
        if initial is not None:
            env_file.write_text(initial)
        self.env.update(DATA_DIR=str(data), BACKUP_DIR=str(data / "backups"), ENV_FILE=str(env_file),
                        PORT="10180", VAULT_NAME="test-vault", CENTER_URL="https://vc.example:10181",
                        TLS_DIR=str(data / "tls"), TRUSTED_IPS="192.0.2.42", BULK_PATHS="",
                        VEILKEY_PASSWORD="test-only-transient-value", SSL_CERT_FILE="/public/verified-ca.pem")
        harness = self.work / "config.sh"
        harness.write_text("set -euo pipefail\n# --- [6/10]" + section)
        self.run_shell([str(harness)])
        return env_file.read_text()

    def test_proxmox_new_env_never_persists_master_password(self):
        text = self.proxmox_config()
        self.assertNotIn("test-only-transient-value", text)
        self.assertNotIn("VEILKEY_UNLOCK_PASSWORD=", text)
        self.assertIn("VEILKEY_TLS_INSECURE=0\n", text)
        self.assertIn("VEILKEY_TRUSTED_IPS=192.0.2.42\n", text)

    def test_proxmox_update_removes_legacy_passwords_and_broad_trust(self):
        text = self.proxmox_config(
            "VEILKEY_UNLOCK_PASSWORD=test-only-transient-value\n"
            "export VEILKEY_PASSWORD=test-only-transient-value\n"
            "VEILKEY_PASSWORD_FILE=/legacy/path\n"
            "VEILKEY_TRUSTED_IPS=10.0.0.0/8,127.0.0.1,::1\n"
            "VEILKEY_TLS_INSECURE=1\nVEILKEY_LABEL=preserved\n")
        self.assertNotIn("test-only-transient-value", text)
        self.assertNotIn("PASSWORD", text)
        self.assertNotIn("10.0.0.0/8", text)
        self.assertIn("VEILKEY_LABEL=preserved\n", text)
        self.assertIn("VEILKEY_TRUSTED_IPS=192.0.2.42\n", text)
        self.assertIn("VEILKEY_TLS_INSECURE=0\n", text)
        self.assertIn("SSL_CERT_FILE=/public/verified-ca.pem\n", text)

    def test_proxmox_first_init_uses_token_not_master_password(self):
        text = (ROOT / "install/proxmox-lxc-debian/install-localvault.sh").read_text()
        self.assertIn('--token "$VEILKEY_REG_TOKEN"', text)
        self.assertNotIn('echo "$VEILKEY_PASSWORD"', text)
        self.assertNotIn('read -s -p "Master password:', text)

    def test_trusted_default_resolver_excludes_loopback(self):
        helper = ROOT / "install/common/localvault-trusted-ip.sh"
        self.assertTrue(helper.is_file())
        self.stub("getent", "printf '127.0.0.1 STREAM localhost\\n::1 STREAM localhost\\n192.0.2.42 STREAM vc.example\\n'")
        result = self.run_shell(["-c", 'source "$1"; veilkey_center_ip "$2"', "test", str(helper), "https://vc.example:10181"])
        self.assertEqual(result.stdout.strip(), "192.0.2.42")
        self.stub("getent", "printf '127.0.0.1 STREAM localhost\\n::1 STREAM localhost\\n'")
        result = subprocess.run(["bash", "-c", 'source "$1"; veilkey_center_ip "$2"', "test", str(helper), "https://localhost:10181"], env=self.env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")

    def test_trusted_default_resolver_supports_macos_without_getent(self):
        helper = ROOT / "install/common/localvault-trusted-ip.sh"
        (self.bin / "getent").unlink()
        (self.bin / "awk").symlink_to("/usr/bin/awk")
        self.stub("dscacheutil", "printf 'ip_address: 127.0.0.1\\nipv6_address: ::1\\nip_address: 192.0.2.42\\n'")
        env = dict(self.env, PATH=str(self.bin))
        result = subprocess.run(["/bin/bash", "-c", 'source "$1"; veilkey_center_ip "$2"', "test", str(helper), "https://vc.example:10181"], env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), "192.0.2.42")

    def test_windows_health_keeps_exception_inside_container(self):
        text = (ROOT / "install/windows/install-veilkey.ps1").read_text()
        self.assertNotIn("TrustAllCerts", text)
        self.assertNotIn("SkipCertificateCheck", text)
        self.assertIn("exec -T vaultcenter", text)
        self.assertIn("curl.exe -sS --cacert", text)

    def test_registration_docs_require_tokens_even_for_trusted_ips(self):
        text = (ROOT / "docs/setup/localvault/registration.md").read_text()
        self.assertNotIn("registration tokens are not required", text)
        self.assertNotIn("Without token", text)
        self.assertIn("Every first registration requires", text)
        self.assertIn("SSL_CERT_FILE", text)
        self.assertIn("--token", text)

    def test_ram_disk_password_helper_is_disabled(self):
        script = ROOT / "install/macos/vaultcenter/setup-tmpfs-password.sh"
        text = script.read_text()
        self.assertNotIn("hdiutil attach", text)
        self.assertNotIn('cp "$DISK_PW"', text)
        result = subprocess.run(["bash", str(script)], env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("no longer supported", result.stderr)
        self.assertEqual(list(self.home.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
