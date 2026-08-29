package main

import (
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStoreHourly(t *testing.T) {
	s := testStore(t)
	now := time.Date(2026, 8, 28, 14, 30, 0, 0, time.UTC)

	rows := []result{
		{Service: "a", Status: statusUp, LatencyMS: 10, CheckedAt: now},
		{Service: "a", Status: statusDown, Detail: "HTTP 500", LatencyMS: 0, CheckedAt: now.Add(5 * time.Minute)},
		{Service: "a", Status: statusUnstable, LatencyMS: 20, CheckedAt: now.Add(-2 * time.Hour)},
		{Service: "b", Status: statusUp, LatencyMS: 5, CheckedAt: now},
		{Service: "old", Status: statusUp, LatencyMS: 5, CheckedAt: now.AddDate(0, 0, -8)},
	}
	for _, r := range rows {
		err := s.insert(r)
		if err != nil {
			t.Fatal(err)
		}
	}

	agg, err := s.hourly(now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatal(err)
	}
	if len(agg) != 2 {
		t.Fatalf("services = %v", agg)
	}

	c := agg["a"]["2026-08-28T14"]
	if c.Up != 1 || c.Unstable != 0 || c.Down != 1 {
		t.Errorf("a 14h = %+v", c)
	}
	c = agg["a"]["2026-08-28T12"]
	if c.Up != 0 || c.Unstable != 1 || c.Down != 0 {
		t.Errorf("a 12h = %+v", c)
	}
	c = agg["b"]["2026-08-28T14"]
	if c.Up != 1 {
		t.Errorf("b 14h = %+v", c)
	}
}

func TestStorePrune(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()

	for _, r := range []result{
		{Service: "a", Status: statusUp, CheckedAt: now},
		{Service: "a", Status: statusUp, CheckedAt: now.AddDate(0, 0, -retentionDays-1)},
	} {
		err := s.insert(r)
		if err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.prune(now.AddDate(0, 0, -retentionDays))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned = %d, want 1", n)
	}
}

func TestStoreRejectsBadStatus(t *testing.T) {
	s := testStore(t)
	err := s.insert(result{Service: "a", Status: "weird", CheckedAt: time.Now().UTC()})
	if err == nil {
		t.Error("want CHECK constraint error")
	}
}
