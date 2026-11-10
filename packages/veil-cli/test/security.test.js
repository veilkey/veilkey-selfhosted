const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const vm = require("node:vm");
const { test } = require("node:test");
const { loadEnv } = require("../bin/env");

const source = fs.readFileSync(process.env.VEIL_WRAPPER_SOURCE || path.join(__dirname, "../bin/veil.js"), "utf8");

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "veil-wrapper-test-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const home = path.join(root, "home");
  const project = path.join(root, "project");
  fs.mkdirSync(home);
  fs.mkdirSync(project);
  return { root, home, project };
}

function run({ root, home, project }, args, options = {}) {
  const calls = [];
  const env = { HOME: home, PATH: "/usr/bin:/bin", VEILKEY_LOCALVAULT_URL: "https://operator.invalid", VEILKEY_VAULTCENTER_URL: "https://center.invalid", VEILKEY_TLS_INSECURE: "0" };
  const processStub = { env, argv: ["node", "veil.js", ...args], cwd: () => project,
    exit(code) { throw new Error(`exit ${code}`); } };
  const fsStub = options.fs || fs;
  const requires = {
    child_process: {
      execFileSync(bin, argv) { calls.push({ bin, argv }); if (options.onExec) options.onExec(bin, argv, env); },
      execSync(command) { calls.push({ command }); return "0.2.0"; },
    },
    "./env": { loadEnv: () => loadEnv(env) },
    "../package.json": { version: "0.2.0" },
    path, fs: fsStub, os: { tmpdir: () => root },
  };
  vm.runInNewContext(source, { require: name => requires[name], __dirname: path.join(__dirname, "../bin"), process: processStub, console: { log() {}, error() {} } });
  return { calls, env, process: processStub };
}

test("launcher makes no registry request or update subprocess", t => {
  const result = run(fixture(t), ["help"]);
  assert.equal(result.calls.length, 0);
  assert.doesNotMatch(source, /npm\s+(?:view|install)/);
});

test("project and parent env files cannot change security settings", t => {
  const data = fixture(t);
  fs.mkdirSync(path.join(data.project, ".veilkey"));
  fs.writeFileSync(path.join(data.project, ".veilkey/env"), "VEILKEY_LOCALVAULT_URL=https://project.invalid\nVEILKEY_VAULTCENTER_URL=https://project.invalid\nVEILKEY_TLS_INSECURE=1\nVEILKEY_CLI_BIN=/project/binary\nPATH=/project/bin\nVEILKEY_PASSWORD_FILE=/project/password\n");
  const nested = path.join(data.project, "nested");
  fs.mkdirSync(nested);
  for (const project of [data.project, nested]) {
    const { env } = run({ ...data, project }, ["status"]);
    assert.equal(env.VEILKEY_API, "https://operator.invalid");
    assert.equal(env.VEILKEY_VAULTCENTER_URL, "https://center.invalid");
    assert.equal(env.VEILKEY_TLS_INSECURE, "0");
    assert.equal(env.PATH, "/usr/bin:/bin");
    assert.equal(env.VEILKEY_PASSWORD_FILE, undefined);
  }
});

test("home and explicitly selected env files remain supported", t => {
  const data = fixture(t);
  fs.mkdirSync(path.join(data.home, ".veilkey"));
  fs.writeFileSync(path.join(data.home, ".veilkey/env"), "VEILKEY_LOCALVAULT_URL=https://home.invalid\n");
  assert.equal(run(data, ["status"]).env.VEILKEY_API, "https://home.invalid");
  const explicit = path.join(data.root, "operator.env");
  fs.writeFileSync(explicit, "export VEILKEY_LOCALVAULT_URL=https://explicit.invalid\n");
  const env = { HOME: data.home, VEILKEY_ENV: explicit };
  loadEnv(env);
  assert.equal(env.VEILKEY_LOCALVAULT_URL, "https://explicit.invalid");
});

test("shell rcfile is private unique and cleaned up", t => {
  const data = fixture(t);
  const paths = [];
  for (let i = 0; i < 2; i++) {
    run(data, [], { onExec(_bin, args) {
      const rc = args.at(-1);
      paths.push(rc);
      assert.equal(fs.statSync(rc).mode & 0o777, 0o600);
      assert.equal(fs.statSync(path.dirname(rc)).mode & 0o777, 0o700);
      assert.match(fs.readFileSync(rc, "utf8"), /unset HISTFILE/);
    } });
  }
  assert.notEqual(paths[0], paths[1]);
  for (const rc of paths) assert.equal(fs.existsSync(rc), false);
});

test("rcfile write failure never starts a shell and removes private temp directory", t => {
  const data = fixture(t);
  const failedFs = { ...fs, writeFileSync() { throw Object.assign(new Error("test write failure"), { code: "EACCES" }); } };
  const result = run(data, [], { fs: failedFs });
  assert.equal(result.calls.length, 0);
  assert.equal(result.process.exitCode, 1);
  assert.equal(fs.readdirSync(data.root).some(name => name.startsWith("veil-shell-")), false);
});
