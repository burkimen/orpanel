package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func pointProbeAt(url string) func() {
	old := omniHealthURL
	omniHealthURL = url
	return func() { omniHealthURL = old }
}

func TestProbeClassification(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		sleep      time.Duration
		want       probeOutcome
		wantStatus int
	}{
		{
			name:       "full json healthy",
			status:     200,
			body:       `{"status":"healthy","version":"3.8.49","uptime":2168.3,"activeConnections":75,"circuitBreakers":{"open":0},"memoryUsage":{"rss":1}}`,
			want:       probeHealthy,
			wantStatus: 200,
		},
		{
			name:       "empty object healthy",
			status:     200,
			body:       `{}`,
			want:       probeHealthy,
			wantStatus: 200,
		},
		{
			name:       "internal error degraded",
			status:     500,
			body:       `{"error":"boom"}`,
			want:       probeDegraded,
			wantStatus: 500,
		},
		{
			name:       "unauthorized degraded",
			status:     401,
			body:       `{"error":"unauthorized"}`,
			want:       probeDegraded,
			wantStatus: 401,
		},
		{
			name:       "html body degraded",
			status:     200,
			body:       `<html>oops</html>`,
			want:       probeDegraded,
			wantStatus: 200,
		},
		{
			name:       "plain text degraded",
			status:     200,
			body:       `not json at all`,
			want:       probeDegraded,
			wantStatus: 200,
		},
		{
			name:   "slow server down",
			status: 200,
			body:   `{}`,
			sleep:  300 * time.Millisecond,
			want:   probeDown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("User-Agent"); got != "orpanel-watchdog" {
					t.Errorf("User-Agent=%q", got)
				}
				if tc.sleep > 0 {
					time.Sleep(tc.sleep)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			defer pointProbeAt(srv.URL)()

			timeout := 1500 * time.Millisecond
			if tc.sleep > 0 {
				timeout = 100 * time.Millisecond
			}
			got := probeOmniHealth(timeout)
			if got.outcome != tc.want {
				t.Fatalf("outcome=%v want %v (err=%v)", got.outcome, tc.want, got.err)
			}
			if tc.wantStatus != 0 && got.statusCode != tc.wantStatus {
				t.Fatalf("status=%d want %d", got.statusCode, tc.wantStatus)
			}
		})
	}
}

func TestProbeClosedPortUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	defer pointProbeAt(url)()

	got := probeOmniHealth(500 * time.Millisecond)
	if got.outcome != probeDown {
		t.Fatalf("outcome=%v want down", got.outcome)
	}
}
