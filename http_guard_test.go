package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuardPolicy(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := guardMiddleware(inner)

	tests := []struct {
		name      string
		host      string
		origin    string
		want      int
		wantInner bool
	}{
		{name: "localhost no origin allowed", host: "localhost:20127", want: http.StatusOK, wantInner: true},
		{name: "loopback no origin allowed", host: "127.0.0.1:20127", want: http.StatusOK, wantInner: true},
		{name: "ipv6 loopback allowed", host: "[::1]:20127", want: http.StatusOK, wantInner: true},
		{name: "same origin allowed", host: "127.0.0.1:20127", origin: "http://127.0.0.1:20127", want: http.StatusOK, wantInner: true},
		{name: "localhost origin allowed", host: "localhost:20127", origin: "http://localhost:20127", want: http.StatusOK, wantInner: true},
		{name: "ipv6 origin allowed", host: "[::1]:20127", origin: "http://[::1]:20127", want: http.StatusOK, wantInner: true},
		{name: "evil host denied", host: "evil.com", want: http.StatusForbidden, wantInner: false},
		{name: "evil host with port denied", host: "evil.com:20127", want: http.StatusForbidden, wantInner: false},
		{name: "evil origin denied", host: "127.0.0.1:20127", origin: "https://evil.com", want: http.StatusForbidden, wantInner: false},
		{name: "null origin denied", host: "127.0.0.1:20127", origin: "null", want: http.StatusForbidden, wantInner: false},
		{name: "non-loopback origin denied", host: "127.0.0.1:20127", origin: "http://192.168.1.5:3000", want: http.StatusForbidden, wantInner: false},
		{name: "localhost wrong port denied", host: "localhost:20127", origin: "http://localhost:9999", want: http.StatusForbidden, wantInner: false},
		{name: "localhost dev server denied", host: "localhost:20127", origin: "http://localhost:3000", want: http.StatusForbidden, wantInner: false},
		{name: "origin without port denied", host: "127.0.0.1:20127", origin: "http://127.0.0.1", want: http.StatusForbidden, wantInner: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got status %d, want %d", rec.Code, tc.want)
			}
			if called != tc.wantInner {
				t.Fatalf("inner invoked=%v, want %v", called, tc.wantInner)
			}
		})
	}
}
