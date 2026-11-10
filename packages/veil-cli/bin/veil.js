#!/usr/bin/env node
const { execFileSync } = require("child_process");
const { loadEnv } = require("./env");
const path = require("path");
const fs = require("fs");
const os = require("os");

const cliBin = path.join(__dirname, "..", "native", "veilkey-cli");

loadEnv();
process.env.VEILKEY_VEIL = "1";
process.env.VEILKEY_CLI_BIN = cliBin;
if (process.env.VEILKEY_LOCALVAULT_URL) {
  process.env.VEILKEY_API = process.env.VEILKEY_LOCALVAULT_URL;
}

const args = process.argv.slice(2);

if (args.length === 0) {
  let rcDir;
  let exitCode = 0;
  try {
    rcDir = fs.mkdtempSync(path.join(os.tmpdir(), "veil-shell-"));
    fs.chmodSync(rcDir, 0o700);
    const rcPath = path.join(rcDir, "bashrc");
    fs.writeFileSync(rcPath, `
[ -f ~/.bashrc ] && source ~/.bashrc
[ -f ~/.bash_profile ] && source ~/.bash_profile
export PS1="\\[\\033[36m\\](VEIL)\\[\\033[0m\\] \\h:\\W \\u\\$ "
unset HISTFILE
`, { flag: "wx", mode: 0o600 });
    execFileSync(cliBin, ["wrap-pty", "bash", "--rcfile", rcPath], { stdio: "inherit" });
  } catch (e) {
    if (e.status === null || e.signal === "SIGKILL") {
      console.error("[veilkey] Binary was killed by macOS. Run:");
      console.error("  sudo codesign --force --sign -", cliBin);
    } else {
      console.error("[veilkey] Protected shell failed:", e.code || e.status || "unknown error");
    }
    exitCode = e.status || 1;
  } finally {
    if (rcDir) fs.rmSync(rcDir, { recursive: true, force: true });
  }
  process.exitCode = exitCode;
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

  try {
    if (Object.hasOwn(map, cmd)) {
      map[cmd]();
    } else {
      execFileSync(cliBin, ["wrap-pty", ...args], { stdio: "inherit" });
    }
  } catch (e) {
    process.exitCode = e.status || 1;
  }
}
