//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// setAutoStart persists the toggle and writes/removes the Run value.
func setAutoStart(enable bool) error {
	cfg := loadConfig()
	saveConfig(cfg.Language, enable)

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer k.Close()

	if enable {
		exePath, _ := os.Executable()
		return k.SetStringValue(AppName, autoStartCommandLine(exePath))
	}
	// Disabling twice is harmless: a missing value is not an error.
	if err := k.DeleteValue(AppName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func isAutoStartEnabled() bool {
	cfg := loadConfig()
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return cfg.AutoStart
	}
	defer k.Close()
	_, _, err = k.GetStringValue(AppName)
	return err == nil
}
