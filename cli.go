package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/getlantern/systray"
)

func showBanner() {
	fmt.Println(strings.Repeat("=", 52))
	fmt.Printf("  OrPanel Control Panel (v%s)\n", getAppVersion())
	fmt.Println(strings.Repeat("=", 52))
	fmt.Println()
}

func showMenu() {
	fmt.Println("  Choose an option:")
	fmt.Println()
	fmt.Println("  1) Web UI (Open in Browser)")
	fmt.Println("  2) Hide to Tray (Background)")
	fmt.Println("  3) Exit")
	fmt.Println()
	fmt.Print("  > ")
}

func runCLI() {
	showBanner()

	cfg := loadConfig()
	t := loadTranslations(cfg.Language)
	srvURL := "http://localhost:20127"
	omniURL := "http://localhost:20128"

	fmt.Printf("  Server: %s\n", srvURL)
	fmt.Println()
	showMenu()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			fmt.Println()
			fmt.Printf("  Opening %s in your browser...\n", srvURL)
			openBrowser(srvURL)
			fmt.Println("  Browser opened. Server is running in background.")
			fmt.Println("  Press Enter to return to menu, or type 'exit' to quit.")
			scanner.Scan()
			if strings.TrimSpace(scanner.Text()) == "exit" {
				return
			}
			fmt.Println()
			showMenu()
		case "2":
			fmt.Println()
			fmt.Printf("  Starting tray icon in background...\n")
			fmt.Printf("  Panel:   %s\n", srvURL)
			fmt.Printf("  OmniRoute: %s\n", omniURL)
			fmt.Println("  The app will run in the system tray.")
			fmt.Println("  Close this terminal window — the app stays in the tray.")
			fmt.Println()
			startWatchdog()
			go startWebServer()
			systray.Run(onReady, onExit)
			return
		case "3", "exit", "quit", "q":
			fmt.Println()
			fmt.Printf("  %s\n", t["TrayQuit"])
			return
		default:
			fmt.Println()
			fmt.Println("  Invalid option. Please choose 1, 2, or 3.")
			fmt.Println()
			showMenu()
		}
	}
}
