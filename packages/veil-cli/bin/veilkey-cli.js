#!/usr/bin/env node
const { execFileSync } = require("child_process");
const path = require("path");

const cliBin = path.join(__dirname, "..", "native", "veilkey-cli");
const args = process.argv.slice(2);

try {
  execFileSync(cliBin, args, { stdio: "inherit" });
} catch (e) {
  process.exit(e.status || 1);
}
