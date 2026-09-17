package main

// autoStartCommandLine renders the Windows Run value data: quoted exe path
// plus --tray, so a logon launch goes straight to tray mode instead of
// depending on stdin-not-a-terminal detection. Quoting handles paths
// containing spaces.
func autoStartCommandLine(exePath string) string {
	return `"` + exePath + `" --tray`
}

// plistContent renders the macOS LaunchAgent plist: exe path plus --tray so a
// logon launch goes straight to tray mode. RunAtLoad starts it at logon.
func plistContent(exePath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.burkimen.orpanel</string>
<key>ProgramArguments</key><array><string>` + exePath + `</string><string>--tray</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><false/>
</dict></plist>
`
}

// desktopContent renders the Linux autostart entry: quoted exe path plus
// --tray, never in a terminal.
func desktopContent(exePath string) string {
	return "[Desktop Entry]\nType=Application\nName=Orpanel\nExec=\"" + exePath + "\" --tray\nTerminal=false\nHidden=false\nNoDisplay=false\nX-GNOME-Autostart-enabled=true\n"
}
