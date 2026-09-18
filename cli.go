package main

import (
	"fmt"
	"strings"
)

func showBanner() {
	fmt.Println(strings.Repeat("=", 52))
	fmt.Printf("  OrPanel Control Panel (v%s)\n", AppVersion)
	fmt.Println(strings.Repeat("=", 52))
	fmt.Println()
}
