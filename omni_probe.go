package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

type probeOutcome int

const (
	probeHealthy probeOutcome = iota
	probeDegraded
	probeDown
)

type probeResult struct {
	outcome    probeOutcome
	statusCode int
	err        error
}

var omniHealthURL = "http://127.0.0.1:" + strconv.Itoa(OmniPort) + "/api/monitoring/health"

func probeOmniHealth(timeout time.Duration) probeResult {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, omniHealthURL, nil)
	if err != nil {
		return probeResult{outcome: probeDown, err: err}
	}
	req.Header.Set("User-Agent", "orpanel-watchdog")
	resp, err := client.Do(req)
	if err != nil {
		return probeResult{outcome: probeDown, err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return probeResult{outcome: probeDown, err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return probeResult{outcome: probeDegraded, statusCode: resp.StatusCode}
	}
	// Tolerant parse: any valid JSON object counts; missing fields stay healthy.
	var v map[string]json.RawMessage
	if err := json.Unmarshal(body, &v); err != nil {
		return probeResult{outcome: probeDegraded, statusCode: resp.StatusCode}
	}
	return probeResult{outcome: probeHealthy, statusCode: resp.StatusCode}
}
