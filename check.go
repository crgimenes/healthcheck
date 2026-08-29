package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	statusUp       = "up"
	statusUnstable = "unstable"
	statusDown     = "down"

	checkTimeout = 30 * time.Second
	pingCount    = "3"
)

type result struct {
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail,omitempty"`
	LatencyMS int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
}

func runCheck(ctx context.Context, c Check) result {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	start := time.Now()
	r := result{Service: c.Name, CheckedAt: start.UTC()}
	switch c.Kind {
	case "http":
		r.Status, r.Detail = checkHTTP(ctx, c.Target)
		r.LatencyMS = time.Since(start).Milliseconds()
	case "ping":
		r.Status, r.Detail, r.LatencyMS = checkPing(ctx, c.Target)
	}
	return r
}

func checkHTTP(ctx context.Context, url string) (status string, detail string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return statusDown, err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return statusDown, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.CopyN(io.Discard, resp.Body, 64*1024)

	if resp.StatusCode != http.StatusOK {
		return statusDown, "HTTP " + strconv.Itoa(resp.StatusCode)
	}
	return statusUp, ""
}

func checkPing(ctx context.Context, host string) (status string, detail string, latencyMS int64) {
	out, err := exec.CommandContext(ctx, "ping", "-c", pingCount, host).CombinedOutput() // #nosec G204 -- host comes from the operator's own config file
	loss, avgMS, parseErr := parsePing(string(out))
	if parseErr != nil {
		detail = parseErr.Error()
		if err != nil {
			detail = firstLine(string(out))
		}
		return statusDown, detail, 0
	}

	latencyMS = int64(avgMS)
	switch {
	case loss <= 0:
		return statusUp, "", latencyMS
	case loss >= 100:
		return statusDown, "100% packet loss", 0
	default:
		return statusUnstable, fmt.Sprintf("%.0f%% packet loss", loss), latencyMS
	}
}

var (
	pingLossRE = regexp.MustCompile(`([0-9.]+)% packet loss`)
	pingRTTRE  = regexp.MustCompile(`min/avg/max[^=]*= *[0-9.]+/([0-9.]+)/`)
)

func parsePing(out string) (loss float64, avgMS float64, err error) {
	m := pingLossRE.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, fmt.Errorf("no packet loss line in ping output")
	}
	loss, err = strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad packet loss %q", m[1])
	}

	m = pingRTTRE.FindStringSubmatch(out)
	if m != nil {
		avgMS, _ = strconv.ParseFloat(m[1], 64)
	}
	return loss, avgMS, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	line, _, _ := strings.Cut(s, "\n")
	if len(line) > 200 {
		line = line[:200]
	}
	if line == "" {
		line = "ping failed with no output"
	}
	return line
}
