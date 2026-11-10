const fs = require("fs");
const path = require("path");

// Only load operator-selected configuration, never files discovered in a project.
function loadEnv(env = process.env) {
  const file = env.VEILKEY_ENV || (env.HOME && path.join(env.HOME, ".veilkey", "env"));
  if (!file || !fs.existsSync(file)) return;
  const content = fs.readFileSync(file, "utf8");
  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const clean = trimmed.replace(/^export\s+/, "");
    const eq = clean.indexOf("=");
    if (eq <= 0) continue;
    const key = clean.slice(0, eq).trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue;
    const value = clean.slice(eq + 1).trim().replace(/^["']|["']$/g, "");
    env[key] = value;
  }
}

module.exports = { loadEnv };
