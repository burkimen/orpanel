#!/usr/bin/env node
// npm postinstall: verify binary exists, chmod +x on unix
const { existsSync, chmodSync } = require("fs");
const { join } = require("path");
const os = require("os");

if (os.platform() !== "win32") {
  const candidates = [
    join(__dirname, "..", "dist", `orpanel-${os.platform()}-${os.arch()}`),
    join(__dirname, "..", "orPanel"),
    join(__dirname, "..", "orpanel"),
  ];
  for (const p of candidates) {
    if (existsSync(p)) {
      try { chmodSync(p, 0o755); } catch {}
      break;
    }
  }
}
console.log("orpanel: postinstall ok — run `orpanel --help` or `orpanel` to start GUI");
