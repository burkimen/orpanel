package main

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func normalizeGuardHost(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimSuffix(h, ".")
	h = strings.Trim(h, "[]")
	return h
}

func splitGuardHost(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return normalizeGuardHost(h)
	}
	// No port present (curl may send bare host).
	if i := strings.LastIndex(hostport, ":"); i != -1 {
		if strings.Count(hostport, ":") == 1 {
			return normalizeGuardHost(hostport[:i])
		}
	}
	return normalizeGuardHost(hostport)
}

func isAllowedGuardHost(h string) bool {
	switch normalizeGuardHost(h) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func guardOriginPortAllowed(hostport string) bool {
	_, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	return port == strconv.Itoa(PanelPort)
}

func guardOriginAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Host == "" {
		return false
	}
	if !isAllowedGuardHost(splitGuardHost(u.Host)) {
		return false
	}
	return guardOriginPortAllowed(u.Host)
}

// guardRequest reports whether the request passes the loopback policy.
func guardRequest(r *http.Request) bool {
	if !isAllowedGuardHost(splitGuardHost(r.Host)) {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && !guardOriginAllowed(origin) {
		return false
	}
	return true
}

func guardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !guardRequest(r) {
			writeLog("WARN: blocked request %s %s host=%q origin=%q", r.Method, r.URL.Path, r.Host, r.Header.Get("Origin"))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
