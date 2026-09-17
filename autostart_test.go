package main

import (
	"strings"
	"testing"
)

func TestAutoStartCommandLine(t *testing.T) {
	tests := []struct {
		name string
		exe  string
	}{
		{"plain path", `C:\Tools\orpanel.exe`},
		{"path with spaces", `C:\Program Files\Orpanel\orPanel.exe`},
		{"unix path", `/home/user/.local/bin/orpanel`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := autoStartCommandLine(tc.exe)
			want := `"` + tc.exe + `" --tray`
			if got != want {
				t.Fatalf("autoStartCommandLine(%q) = %q, want %q", tc.exe, got, want)
			}
			if strings.Count(got, `"`+tc.exe+`"`) != 1 {
				t.Fatalf("exe path not quoted exactly once in %q", got)
			}
			if strings.HasPrefix(got, `""`) || strings.Contains(got, `""`) {
				t.Fatalf("double-quoted incorrectly: %q", got)
			}
		})
	}
}

func TestPlistContent(t *testing.T) {
	for _, exe := range []string{`/usr/local/bin/orpanel`, `/Applications/My Apps/orpanel`} {
		got := plistContent(exe)
		if !strings.Contains(got, "<string>"+exe+"</string>") {
			t.Errorf("plist missing exe entry for %q:\n%s", exe, got)
		}
		if !strings.Contains(got, "<string>--tray</string>") {
			t.Errorf("plist missing --tray entry:\n%s", got)
		}
		if !strings.Contains(got, "<key>RunAtLoad</key><true/>") {
			t.Errorf("plist missing RunAtLoad:\n%s", got)
		}
	}
}

func TestDesktopContent(t *testing.T) {
	for _, exe := range []string{`/home/user/.local/bin/orpanel`, `/home/user/My Apps/orpanel`} {
		got := desktopContent(exe)
		wantExec := `Exec="` + exe + `" --tray`
		if !strings.Contains(got, wantExec) {
			t.Errorf("desktop missing %q:\n%s", wantExec, got)
		}
		if !strings.Contains(got, "Terminal=false") {
			t.Errorf("desktop missing Terminal=false:\n%s", got)
		}
		if !strings.Contains(got, "X-GNOME-Autostart-enabled=true") {
			t.Errorf("desktop missing X-GNOME-Autostart-enabled:\n%s", got)
		}
	}
}
