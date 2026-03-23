#!/usr/bin/env node
const { execFileSync, execSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const nativeDir = path.join(__dirname, "..", "native");
const veilBin = path.join(nativeDir, "veil");
const cliBin = path.join(nativeDir, "veilkey-cli");
const pkg = require("../package.json");

// Check for updates (non-blocking, silent on error)
try {
  const latest = execSync("npm view veilkey-cli version 2>/dev/null", { encoding: "utf8", timeout: 3000 }).trim();
  if (latest && latest !== pkg.version) {
    console.log(`\x1b[33m[veilkey] 새 버전 ${latest} 사용 가능 (현재 ${pkg.version})\x1b[0m`);
    console.log(`\x1b[33m  npm install -g veilkey-cli\x1b[0m`);
    console.log("");
  }
} catch (_) {}

// Auto-load .veilkey/env
function loadEnv() {
  let dir = process.cwd();
  while (dir !== "/") {
    const envFile = path.join(dir, ".veilkey", "env");
    if (fs.existsSync(envFile)) {
      const content = fs.readFileSync(envFile, "utf8");
      for (const line of content.split("\n")) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith("#")) continue;
        const clean = trimmed.replace(/^export\s+/, "");
        const eq = clean.indexOf("=");
        if (eq > 0) {
          const k = clean.slice(0, eq).trim();
          const v = clean.slice(eq + 1).trim().replace(/^["']|["']$/g, "");
          process.env[k] = v;
        }
      }
      return;
    }
    dir = path.dirname(dir);
  }
  // Fallback
  const home = process.env.HOME || "";
  for (const sub of [".veilkey/env", "veilkey-selfhosted/.veilkey/env"]) {
    const p = path.join(home, sub);
    if (fs.existsSync(p)) {
      loadEnvFile(p);
      return;
    }
  }
}

function loadEnvFile(filePath) {
  const content = fs.readFileSync(filePath, "utf8");
  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const clean = trimmed.replace(/^export\s+/, "");
    const eq = clean.indexOf("=");
    if (eq > 0) {
      const k = clean.slice(0, eq).trim();
      const v = clean.slice(eq + 1).trim().replace(/^["']|["']$/g, "");
      process.env[k] = v;
    }
  }
}

loadEnv();
process.env.VEILKEY_CLI_BIN = cliBin;
if (process.env.VEILKEY_LOCALVAULT_URL) {
  process.env.VEILKEY_API = process.env.VEILKEY_LOCALVAULT_URL;
}

const args = process.argv.slice(2);

if (args.length === 0) {
  // Create temp rcfile for custom prompt
  const os = require("os");
  const rcPath = path.join(os.tmpdir(), `veil-bashrc-${process.pid}`);
  fs.writeFileSync(rcPath, `
[ -f ~/.bashrc ] && source ~/.bashrc
[ -f ~/.bash_profile ] && source ~/.bash_profile
export PS1="\\[\\033[36m\\](VEIL)\\[\\033[0m\\] \\h:\\W \\u\\$ "
`);
  try {
    execFileSync(cliBin, ["wrap-pty", "bash", "--rcfile", rcPath], { stdio: "inherit" });
  } catch (e) {
    if (e.status === null || e.signal === 'SIGKILL') {
      console.error("[veilkey] Binary was killed by macOS. Run:");
      console.error("  sudo codesign --force --sign -", cliBin);
    } else if (e.status) {
      console.error("[veilkey] exit:", e.status);
    }
    process.exit(e.status || 1);
  } finally {
    try { fs.unlinkSync(rcPath); } catch (_) {}
  }
} else {
  const cmd = args[0];
  const rest = args.slice(1);
  const map = {
    help: () => {
      console.log("Usage:");
      console.log("  veil                     Enter protected session (PTY masking)");
      console.log("  veil status              Show VeilKey connection status");
      console.log("  veil resolve <VK:ref>    Resolve a VK reference");
      console.log("  veil exec <command...>   Resolve VK refs in args and execute");
      console.log("  veil scan [file...]      Scan files for secrets");
      console.log("  veil help                Show this help");
    },
    status: () => execFileSync(cliBin, ["status"], { stdio: "inherit" }),
    resolve: () => execFileSync(cliBin, ["resolve", ...rest], { stdio: "inherit" }),
    exec: () => execFileSync(cliBin, ["exec", ...rest], { stdio: "inherit" }),
    scan: () => execFileSync(cliBin, ["scan", ...rest], { stdio: "inherit" }),
  };

  if (map[cmd]) {
    try { map[cmd](); } catch (e) { process.exit(e.status || 1); }
  } else {
    try {
      execFileSync(cliBin, ["wrap-pty", ...args], { stdio: "inherit" });
    } catch (e) {
      process.exit(e.status || 1);
    }
  }
}
