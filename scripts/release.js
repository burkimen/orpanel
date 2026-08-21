#!/usr/bin/env node
/**
 * orpanel release script
 * Tek komutla: version bump + git commit + tag + push + npm publish
 *
 * Kullanim:
 *   node scripts/release.js patch   # 1.0.0 -> 1.0.1 (hata duzeltme)
 *   node scripts/release.js minor   # 1.0.0 -> 1.1.0 (yeni ozellik)
 *   node scripts/release.js major   # 1.0.0 -> 2.0.0 (buyuk degisiklik)
 */
const { execSync } = require("child_process");
const { readFileSync } = require("fs");
const { join } = require("path");

const bump = process.argv[2] || "patch";
if (!["patch", "minor", "major"].includes(bump)) {
  console.error("Kullanim: node scripts/release.js [patch|minor|major]");
  process.exit(1);
}

const run = (cmd) => {
  console.log(`> ${cmd}`);
  execSync(cmd, { stdio: "inherit", cwd: join(__dirname, "..") });
};

// 1. Version bump (package.json guncelle + git commit + git tag)
console.log("\n=== 1. Version bump ===");
run(`npm version ${bump} --no-git-tag-version`);

const pkg = JSON.parse(readFileSync(join(__dirname, "..", "package.json"), "utf-8"));
const version = pkg.version;
console.log(`Yeni surum: v${version}`);

// 2. Git commit + tag
console.log("\n=== 2. Git commit + tag ===");
run("git add -A");
run(`git commit -m "release: v${version}"`);
run(`git tag v${version}`);

// 3. Git push
console.log("\n=== 3. Git push ===");
run("git push origin master");
run(`git push origin v${version}`);

// 4. npm publish
console.log("\n=== 4. npm publish ===");
run("npm publish");

console.log(`\n=== TAMAMLANDI ===`);
console.log(`npm:    https://www.npmjs.com/package/orpanel/v/${version}`);
console.log(`github: https://github.com/burkimen/orpanel/releases/tag/v${version}`);
console.log(`\nGitHub Actions 4 platform binary'si otomatik derleyecek.`);
