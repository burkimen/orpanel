package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func showBanner() {
	fmt.Println(strings.Repeat("=", 52))
	fmt.Printf("  OrPanel Control Panel (v%s)\n", AppVersion)
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
	_ = cfg
	srvURL := "http://localhost:20127"

	fmt.Printf("  Server: %s\n", srvURL)
	fmt.Println()
	showMenu()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1":
			fmt.Println()
			fmt.Printf("  Opening %s in browser...\n", srvURL)
			openBrowser(srvURL)
			fmt.Println("  Server is running.")
			fmt.Println("  Press Enter to return to menu, or type 'exit' to quit.")
			scanner.Scan()
			if strings.TrimSpace(scanner.Text()) == "exit" {
				return
			}
			fmt.Println()
			showMenu()
		case "2":
			fmt.Println()
			fmt.Println("  Starting background process...")
			spawnDetached()
			fmt.Println("  Done. You can close this terminal.")
			return
		case "3", "exit", "quit", "q":
			return
		default:
			fmt.Println("  Invalid option. Choose 1, 2, or 3.")
			fmt.Println()
			showMenu()
		}
	}
}
