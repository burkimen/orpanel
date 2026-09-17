package main

import (
	"testing"
)

func TestOwnedByOmnirouteTable(t *testing.T) {
	oldPath, oldFn := OmniroutePath, processCommandLineFn
	defer func() { OmniroutePath, processCommandLineFn = oldPath, oldFn }()

	OmniroutePath = `C:\Users\x\AppData\Roaming\npm\node_modules\omniroute`

	tests := []struct {
		name    string
		cmdline string
		ok      bool
		want    bool
	}{
		{
			name:    "entry script",
			cmdline: `node C:\Users\x\AppData\Roaming\npm\node_modules\omniroute\bin\omniroute.mjs --no-open --no-tray`,
			ok:      true,
			want:    true,
		},
		{
			name:    "grep probe",
			cmdline: `grep -r omniroute D:\Projects`,
			ok:      true,
			want:    false,
		},
		{
			name:    "editor notes",
			cmdline: `C:\Program Files\Notepad++\notepad++.exe omniroute-notes.txt`,
			ok:      true,
			want:    false,
		},
		{
			name:    "lookup failure",
			cmdline: "",
			ok:      false,
			want:    false,
		},
		{
			name:    "empty cmdline",
			cmdline: "",
			ok:      true,
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			processCommandLineFn = func(pid int) (string, bool) {
				return tc.cmdline, tc.ok
			}
			if got := ownedByOmniroute(1234); got != tc.want {
				t.Fatalf("ownedByOmniroute(%q)=%v want %v", tc.cmdline, got, tc.want)
			}
		})
	}
}

func TestOwnedByOmnirouteEmptyPath(t *testing.T) {
	oldPath, oldFn := OmniroutePath, processCommandLineFn
	defer func() { OmniroutePath, processCommandLineFn = oldPath, oldFn }()

	OmniroutePath = ""
	processCommandLineFn = func(pid int) (string, bool) {
		return `node C:\x\omniroute\bin\omniroute.mjs`, true
	}
	if ownedByOmniroute(1234) {
		t.Fatal("empty OmniroutePath must refuse attribution")
	}
}
