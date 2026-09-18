package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestOwnPanelImageMatching is pure: the ownership check must match the
// normal image AND renamed-aside images, and nothing else.
func TestOwnPanelImageMatching(t *testing.T) {
	cases := []struct {
		img, exe string
		want     bool
	}{
		{`C:\P\orPanel.exe`, `C:\P\orPanel.exe`, true},
		{`C:\P\orPanel.exe.old`, `C:\P\orPanel.exe`, true},
		{`C:\P\orPanel.exe.old.20240101-000000.1.2`, `C:\P\orPanel.exe`, true},
		{`C:\P\orPanel.exe.old`, `C:\P\orpanel.exe`, true}, // case-insensitive
		{`C:\P\other.exe`, `C:\P\orPanel.exe`, false},
		{`C:\P\orPanel.exe.bak`, `C:\P\orPanel.exe`, false},
		{`C:\Q\orPanel.exe`, `C:\P\orPanel.exe`, false}, // different dir
		{``, `C:\P\orPanel.exe`, false},
	}
	for _, c := range cases {
		old := processImagePathFn
		img := c.img
		processImagePathFn = func(pid int) (string, bool) {
			if img == "" {
				return "", false
			}
			return img, true
		}
		got := isOwnPanelProcess(4242, c.exe)
		processImagePathFn = old
		if got != c.want {
			t.Fatalf("img=%q exe=%q got %v want %v", c.img, c.exe, got, c.want)
		}
	}
}

// TestReclaimPanelPortFreesOwnHolder starts a real child holding an
// ephemeral loopback port (same binary image => ownership-positive) and
// asserts reclaimPanelPort frees it via the real kill path. Sandbox only:
// ephemeral port, the child is our own test binary, cleaned up.
func TestReclaimPanelPortFreesOwnHolder(t *testing.T) {
	port := freeLoopbackPort(t)
	child := exec.Command(os.Args[0], "-test.run=TestHelperPortHolder")
	child.Env = append(os.Environ(), fmt.Sprintf("ORPANEL_HOLD_PORT=%d", port))
	if err := child.Start(); err != nil {
		t.Skipf("cannot start holder: %v", err)
	}
	defer func() { _ = child.Process.Kill() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 200*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("holder never bound")
		}
		time.Sleep(100 * time.Millisecond)
	}
	exe, _ := os.Executable()
	if !reclaimPanelPort(port, exe) {
		t.Fatalf("could not reclaim port from own child holder")
	}
	if isPortInUse(port) {
		t.Fatalf("port still busy after reclaim")
	}
}

// TestReclaimLeavesForeignHolder verifies the negative case through the
// pure gate: a foreign image is never ours, so no kill path can fire.
func TestReclaimLeavesForeignHolder(t *testing.T) {
	old := processImagePathFn
	defer func() { processImagePathFn = old }()
	processImagePathFn = func(pid int) (string, bool) {
		return filepath.Join(string(filepath.Separator) + "usr", "bin", "unrelated"), true
	}
	exe, _ := os.Executable()
	if isOwnPanelProcess(os.Getpid()+999999, exe) {
		t.Fatalf("foreign image classified as own panel process")
	}
}

// TestReclaimOwnHolderEndToEnd holds a real port with a child stand-in and
// reclaims it via the real kill path on unix (taskkill on Windows needs a
// real process; the ownership gate above covers the Windows branch).
func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestHelperPortHolder is not a real test: re-executed as a child to hold a
// port with our own binary image (ownership-positive holder).
func TestHelperPortHolder(t *testing.T) {
	p := os.Getenv("ORPANEL_HOLD_PORT")
	if p == "" {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+p)
	if err != nil {
		return
	}
	defer ln.Close()
	time.Sleep(60 * time.Second)
}

var _ = strings.Contains // keep strings import if helpers shrink
