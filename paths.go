package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var cachedExeDir string

func exeDir() string {
	if cachedExeDir != "" {
		return cachedExeDir
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		// When running via `go run`, exe is in temp; fall back to cwd if locales/themes not found there
		if _, err := os.Stat(filepath.Join(dir, "locales")); err == nil {
			cachedExeDir = dir
			return cachedExeDir
		}
		if _, err := os.Stat(filepath.Join(dir, "themes")); err == nil {
			cachedExeDir = dir
			return cachedExeDir
		}
		// fallback to working dir for dev
		if cwd, err := os.Getwd(); err == nil {
			if _, err := os.Stat(filepath.Join(cwd, "locales")); err == nil {
				cachedExeDir = cwd
				return cachedExeDir
			}
		}
		cachedExeDir = dir
		return cachedExeDir
	}
	if cwd, err := os.Getwd(); err == nil {
		cachedExeDir = cwd
		return cachedExeDir
	}
	cachedExeDir = "."
	return cachedExeDir
}

func getConfigPath() string {
	return filepath.Join(exeDir(), ConfigFileName)
}

func getLogPath() string {
	return filepath.Join(exeDir(), LogFileName)
}

func getLocalesDir() string {
	// try exeDir first, then cwd
	p := filepath.Join(exeDir(), LocalesDir)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if cwd, err := os.Getwd(); err == nil {
		alt := filepath.Join(cwd, LocalesDir)
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return p
}

func getThemesDir() string {
	p := filepath.Join(exeDir(), "themes")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if cwd, err := os.Getwd(); err == nil {
		alt := filepath.Join(cwd, "themes")
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return p
}

func getIconPath() string {
	// Windows prefers ico, Unix prefers png
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{"app.ico", "icon.png"}
	} else {
		candidates = []string{"icon.png", "app.ico"}
	}
	for _, name := range candidates {
		p := filepath.Join(exeDir(), name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if cwd, err := os.Getwd(); err == nil {
			alt := filepath.Join(cwd, name)
			if _, err := os.Stat(alt); err == nil {
				return alt
			}
		}
	}
	// fallback to app.ico in exeDir
	return filepath.Join(exeDir(), "app.ico")
}

// Enhanced OmniRoute path discovery
func getOmniroutePathEnhanced() string {
	candidates := []string{}

	// 1. Original logic
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "npm", "node_modules", "omniroute"))
		}
		if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
			candidates = append(candidates, filepath.Join(programFiles, "nodejs", "node_modules", "npm", "node_modules", "omniroute"))
		}
	} else {
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, ".npm", "lib", "node_modules", "omniroute"))
			candidates = append(candidates, filepath.Join(home, ".npm", "node_modules", "omniroute"))
			// nvm paths
			candidates = append(candidates, filepath.Join(home, ".nvm", "versions", "node"))
			// glob nvm handled separately
		}
		candidates = append(candidates, "/usr/local/lib/node_modules/omniroute")
		candidates = append(candidates, "/opt/homebrew/lib/node_modules/omniroute")
		candidates = append(candidates, "/usr/lib/node_modules/omniroute")
	}

	// 2. npm prefix -g
	if npmPrefix := getNpmPrefix(); npmPrefix != "" {
		// npm prefix is like /usr/local or C:\Users\...\AppData\Roaming\npm
		candidates = append(candidates, filepath.Join(npmPrefix, "lib", "node_modules", "omniroute"))
		candidates = append(candidates, filepath.Join(npmPrefix, "node_modules", "omniroute"))
	}

	// Check candidates
	for _, p := range candidates {
		if isOmnirouteDir(p) {
			return p
		}
		// for nvm base, glob
		if strings.Contains(p, ".nvm") {
			if matches, _ := filepath.Glob(filepath.Join(p, "*", "lib", "node_modules", "omniroute")); matches != nil {
				for _, m := range matches {
					if isOmnirouteDir(m) {
						return m
					}
				}
			}
		}
	}

	// fallback to original
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "npm", "node_modules", "omniroute")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".npm", "lib", "node_modules", "omniroute")
}

func getNpmPrefix() string {
	// timeout quickly
	cmd := exec.Command("npm", "prefix", "-g")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isOmnirouteDir(p string) bool {
	if _, err := os.Stat(filepath.Join(p, "bin", "omniroute.mjs")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(p, "package.json")); err == nil {
		return true
	}
	return false
}
