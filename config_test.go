package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func TestIsVersionLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.1.9", "1.1.10", true},
		{"1.1.10", "1.1.9", false},
		{"1.2.0", "1.2.0", false},
		{"v1.2.0", "1.2.1", true},
		{"2.0", "2.0.1", true},
		{"", "1.0.0", false},
		{"1.0.0", "", false},
	}
	for _, tc := range tests {
		if got := isVersionLess(tc.a, tc.b); got != tc.want {
			t.Errorf("isVersionLess(%q,%q)=%v want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestConfigSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	configPathOverride = filepath.Join(dir, "config.json")
	defer func() { configPathOverride = "" }()

	saveConfig("en", true)
	cfg := loadConfig()
	if cfg.Language != "en" || !cfg.AutoStart {
		t.Fatalf("roundtrip got %+v", cfg)
	}
	if err := saveTheme(ThemeDark); err != nil {
		t.Fatal(err)
	}
	cfg = loadConfig()
	if cfg.Theme != ThemeDark {
		t.Fatalf("theme roundtrip got %q", cfg.Theme)
	}
	if _, err := os.Stat(configPathOverride); err != nil {
		t.Fatal(err)
	}
}

func TestAutostartPostHeadless(t *testing.T) {
	if mAutoStart != nil {
		t.Skip("tray menu present; headless precondition not met")
	}
	oldApply := autostartApply
	oldEnabled := autostartEnabled
	defer func() { autostartApply, autostartEnabled = oldApply, oldEnabled }()
	configPathOverride = filepath.Join(t.TempDir(), "config.json")
	defer func() { configPathOverride = "" }()
	var got []bool
	autostartApply = func(on bool) error { got = append(got, on); return nil }
	autostartEnabled = func() bool { return len(got) > 0 && got[len(got)-1] }
	mux := newPanelMux()
	req := httptest.NewRequest(http.MethodPost, "/api/autostart", strings.NewReader(`{"isEnabled":true}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if len(got) != 1 || !got[0] {
		t.Fatalf("apply calls=%v", got)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/autostart", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	var resp AutoStartResponse
	if err := json.NewDecoder(getRec.Result().Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.IsEnabled {
		t.Fatalf("GET state=%+v", resp)
	}
}

func TestConcurrentAccessorsAndGuard(t *testing.T) {
	inner := guardMiddleware(newTestMux())
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			setCurrentLang("en")
			_ = getCurrentLang()
			setIntentionalStop(n%2 == 0)
			_ = getIntentionalStop()
			writeLog("concurrency probe %d", n)
			req := httptest.NewRequest("GET", "/", nil)
			req.Host = "127.0.0.1:20127"
			rec := httptest.NewRecorder()
			inner.ServeHTTP(rec, req)
		}(i)
	}
	wg.Wait()
}
