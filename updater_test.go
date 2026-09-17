package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shaOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestVerifyChecksumTable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "asset.bin")
	content := "orpanel-update-payload"
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	good := shaOf(content)
	bad := shaOf("tampered")
	other := shaOf("other-file")

	tests := []struct {
		name      string
		sums      string
		asset     string
		wantErr   string // "" = nil; else substring
		lastWins  string // expected hash when duplicate entries used
		checkLast bool
	}{
		{
			name:  "correct line found",
			sums:  good + "  asset.bin\n" + other + "  other.bin\n",
			asset: "asset.bin",
		},
		{
			name:    "missing asset",
			sums:    other + "  other.bin\n",
			asset:   "asset.bin",
			wantErr: "not found",
		},
		{
			name:    "malformed hex line ignored then missing",
			sums:    "zzzz  asset.bin\n",
			asset:   "asset.bin",
			wantErr: "malformed",
		},
		{
			name:  "binary marker form",
			sums:  good + " *asset.bin\n",
			asset: "asset.bin",
		},
		{
			name:  "extra whitespace and blank lines",
			sums:  "\n\n   " + good + "   asset.bin  \n\n" + other + "  other.bin\n",
			asset: "asset.bin",
		},
		{
			name:      "duplicate entries last match wins",
			sums:      bad + "  asset.bin\n" + good + "  asset.bin\n",
			asset:     "asset.bin",
			lastWins:  "last",
			checkLast: true,
		},
		{
			name:    "duplicate entries first entry stale fails",
			sums:    good + "  asset.bin\n" + bad + "  asset.bin\n",
			asset:   "asset.bin",
			wantErr: "mismatch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyChecksum(tc.sums, tc.asset, target)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestVerifyChecksumFileContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "asset.bin")
	content := "known-content-123"
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	good := shaOf(content)
	bad := shaOf("different")

	if err := verifyChecksum(good+"  asset.bin\n", "asset.bin", target); err != nil {
		t.Fatalf("correct checksum rejected: %v", err)
	}

	err := verifyChecksum(bad+"  asset.bin\n", "asset.bin", target)
	if err == nil {
		t.Fatal("tampered checksum accepted")
	}
	if !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), good) {
		t.Fatalf("mismatch error must contain both hashes, got: %v", err)
	}

	if err := verifyChecksum(good+"  asset.bin\n", "asset.bin", filepath.Join(dir, "missing.bin")); err == nil {
		t.Fatal("nonexistent path accepted")
	}
}

func TestGetAssetNameSharedByDownloadURL(t *testing.T) {
	name := getAssetName()
	if name == "" {
		t.Fatal("empty asset name")
	}
	url := getDownloadURL("9.9.9")
	if !strings.HasSuffix(url, "/v9.9.9/"+name) {
		t.Fatalf("download URL %q does not share asset name %q", url, name)
	}
	if strings.Count(url, name) != 1 {
		t.Fatalf("asset name appears %d times in %q", strings.Count(url, name), url)
	}
}
