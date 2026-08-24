package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const ThemeLight = "light"
const ThemeDark = "dark"
const ThemeSystem = "system"

type Config struct {
	Language          string `json:"language"`
	AutoStart         bool   `json:"auto_start"`
	Theme             string `json:"theme"`
	LogRetentionHours int    `json:"log_retention_hours"`
}

func isValidTheme(t string) bool {
	return t == ThemeLight || t == ThemeDark || t == ThemeSystem
}

func loadConfigUnlocked() Config {
	cfgPath := getConfigPath()
	file, err := os.ReadFile(cfgPath)
	if err != nil {
		if cfgPath != ConfigFileName {
			file, err = os.ReadFile(ConfigFileName)
		}
	}
	if err != nil {
		return Config{Language: "", AutoStart: false, Theme: ThemeSystem}
	}
	var cfg Config
	if err := json.Unmarshal(file, &cfg); err != nil {
		return Config{Language: "", AutoStart: false, Theme: ThemeSystem}
	}
	if !isValidTheme(cfg.Theme) {
		cfg.Theme = ThemeSystem
	}
	if cfg.LogRetentionHours <= 0 {
		cfg.LogRetentionHours = 24
	}
	if cfg.Language != "" {
		currentLang = cfg.Language
	}
	return cfg
}

func loadConfig() Config {
	configMutex.Lock()
	defer configMutex.Unlock()
	return loadConfigUnlocked()
}

func saveConfig(lang string, autoStart bool) {
	configMutex.Lock()
	defer configMutex.Unlock()
	existing := loadConfigUnlocked()
	if lang != "" {
		currentLang = lang
		existing.Language = currentLang
	} else if existing.Language == "" {
		existing.Language = currentLang
	}
	existing.AutoStart = autoStart
	if !isValidTheme(existing.Theme) {
		existing.Theme = ThemeSystem
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(getConfigPath(), data, 0644)
}

func saveTheme(theme string) error {
	if !isValidTheme(theme) {
		return fmt.Errorf("invalid theme: %s", theme)
	}
	configMutex.Lock()
	defer configMutex.Unlock()
	cfg := loadConfigUnlocked()
	cfg.Theme = theme
	if cfg.Language == "" {
		cfg.Language = currentLang
	}
	if !isValidTheme(cfg.Theme) {
		cfg.Theme = ThemeSystem
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(getConfigPath(), data, 0644)
}

func loadTranslations(lang string) map[string]string {
	file, err := fs.ReadFile(webFS, "locales/"+lang+".json")
	if err != nil {
		file, err = fs.ReadFile(webFS, "locales/tr.json")
		if err != nil {
			return map[string]string{
				"Title": "Omniroute Control Panel",
			}
		}
	}
	var t map[string]string
	json.Unmarshal(file, &t)
	return t
}

func detectLanguage(acceptLang string) string {
	if acceptLang == "" {
		return "en"
	}
	acceptLang = strings.ToLower(acceptLang)
	for _, supported := range []string{"tr", "en", "es"} {
		if strings.Contains(acceptLang, supported) {
			return supported
		}
	}
	if strings.Contains(acceptLang, "zh") || strings.Contains(acceptLang, "ja") || strings.Contains(acceptLang, "ko") {
		return "en"
	}
	return "en"
}
