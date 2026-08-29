package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParsePing(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		loss    float64
		avgMS   float64
		wantErr bool
	}{
		{
			name: "macos ok",
			out: `3 packets transmitted, 3 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 0.045/0.058/0.075/0.012 ms`,
			loss:  0,
			avgMS: 0.058,
		},
		{
			name: "linux ok",
			out: `3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 11.2/12.5/14.0/1.1 ms`,
			loss:  0,
			avgMS: 12.5,
		},
		{
			name: "partial loss",
			out:  `3 packets transmitted, 2 packets received, 33.3% packet loss`,
			loss: 33.3,
		},
		{
			name: "total loss",
			out:  `3 packets transmitted, 0 packets received, 100.0% packet loss`,
			loss: 100,
		},
		{
			name:    "no loss line",
			out:     `ping: cannot resolve nosuchhost: Unknown host`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loss, avgMS, err := parsePing(tt.out)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if loss != tt.loss || avgMS != tt.avgMS {
				t.Errorf("loss = %v avgMS = %v, want %v %v", loss, avgMS, tt.loss, tt.avgMS)
			}
		})
	}
}

func TestCheckHTTP(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	status, detail := checkHTTP(ctx, ok.URL)
	if status != statusUp || detail != "" {
		t.Errorf("ok: %s %q", status, detail)
	}
	status, detail = checkHTTP(ctx, bad.URL)
	if status != statusDown || detail != "HTTP 500" {
		t.Errorf("bad: %s %q", status, detail)
	}
	status, _ = checkHTTP(ctx, closed.URL)
	if status != statusDown {
		t.Errorf("closed: %s", status)
	}
}

func TestCheckPingLocalhost(t *testing.T) {
	_, err := exec.LookPath("ping")
	if err != nil {
		t.Skip("ping not available")
	}

	ctx, cancel := context.WithTimeout(t.Context(), checkTimeout)
	defer cancel()

	status, detail, _ := checkPing(ctx, "127.0.0.1")
	if status != statusUp {
		if strings.Contains(detail, "not permitted") || strings.Contains(detail, "permission") {
			t.Skipf("ping not permitted: %s", detail)
		}
		t.Errorf("localhost: %s %q", status, detail)
	}

	status, _, _ = checkPing(ctx, "nosuchhost.invalid")
	if status != statusDown {
		t.Errorf("invalid host: %s", status)
	}
}
