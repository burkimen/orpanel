//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

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
		return k.SetStringValue(AppName, `"`+exePath+`"`)
	}
	return k.DeleteValue(AppName)
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
