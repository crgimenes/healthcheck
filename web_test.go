package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebPage(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	for _, r := range []result{
		{Service: "site", Status: statusUp, LatencyMS: 12, CheckedAt: now.Add(-time.Hour)},
		{Service: "site", Status: statusDown, Detail: "HTTP 500", CheckedAt: now},
	} {
		err := s.insert(r)
		if err != nil {
			t.Fatal(err)
		}
	}

	cfg := &Config{
		Checks: []Check{
			{Name: "site", Kind: "http", Target: "https://example.com"},
			{Name: "quiet", Kind: "ping", Target: "10.0.0.1"},
		},
	}
	h := newWebHandler(cfg, s)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"site", "quiet", "cell up", "cell down", "cell nodata", "50.00%", "no data"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if got := strings.Count(body, `class="cell`); got != 2*weekHours {
		t.Errorf("cells = %d, want %d", got, 2*weekHours)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/style.css", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("style.css status = %d", rec.Code)
	}
}
