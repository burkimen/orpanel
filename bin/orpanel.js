#!/usr/bin/env node
/**
 * orpanel npm wrapper — spawns the Go binary for current os/arch.
 * 100% npm uyumlu, standartlara uygun.
 */
const { spawn } = require("child_process");
const { join, dirname } = require("path");
const { existsSync } = require("fs");
const os = require("os");

const ROOT = join(__dirname, "..");
const plat = os.platform(); // win32, darwin, linux
const arch = os.arch(); // x64, arm64

const candidates = [
  join(ROOT, "dist", `orpanel-${plat}-${arch}` + (plat === "win32" ? ".exe" : "")),
  join(ROOT, "dist", `orPanel-${plat}-${arch}` + (plat === "win32" ? ".exe" : "")),
  join(ROOT, plat === "win32" ? "orPanel.exe" : "orPanel"),
  join(ROOT, "orpanel" + (plat === "win32" ? ".exe" : "")),
  // fallback to exeDir technique (for global npm link where dist is flattened)
  join(dirname(__dirname), "..", "..", "bin", `orpanel-${plat}-${arch}`),
];

let bin = null;
for (const p of candidates) {
  if (existsSync(p)) { bin = p; break; }
}

if (!bin) {
  console.error(`orpanel: binary not found for ${plat}/${arch}`);
  console.error(`Tried:\n  ${candidates.join("\n  ")}`);
  console.error(`\nTry reinstall: npm install -g orpanel@latest  or  curl -fsSL https://get.orpanel.dev/install.sh | sh`);
  process.exit(1);
}

// pass through args
const args = process.argv.slice(2);
const child = spawn(bin, args, { stdio: "inherit", cwd: ROOT });
child.on("exit", (code, sig) => {
  if (sig) process.kill(process.pid, sig);
  else process.exit(code ?? 0);
});
child.on("error", (e) => {
  console.error("orpanel spawn error:", e.message);
  process.exit(1);
});
