package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode = WAL;
CREATE TABLE IF NOT EXISTS results (
	id INTEGER PRIMARY KEY,
	service TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('up', 'unstable', 'down')),
	detail TEXT NOT NULL DEFAULT '',
	latency_ms INTEGER NOT NULL,
	checked_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS results_service_time ON results (service, checked_at);
`

type Store struct {
	db *sql.DB
}

func openStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	_, err = db.Exec(schema)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

const sqlInsertResult = `INSERT INTO results (
		service,    -- 1
		status,     -- 2
		detail,     -- 3
		latency_ms, -- 4
		checked_at  -- 5
	) VALUES (
		?, -- 1
		?, -- 2
		?, -- 3
		?, -- 4
		?  -- 5
	);`

func (s *Store) insert(r result) error {
	_, err := s.db.Exec(
		sqlInsertResult,
		r.Service,                        // 1
		r.Status,                         // 2
		r.Detail,                         // 3
		r.LatencyMS,                      // 4
		r.CheckedAt.Format(time.RFC3339), // 5
	)
	return err
}

type hourCount struct {
	Up       int
	Unstable int
	Down     int
}

// The substr length must match the hourKey format in web.go ("2006-01-02T15").
const sqlHourly = `SELECT
		service,                   -- 1
		substr(checked_at, 1, 13), -- 2
		SUM(status = 'up'),        -- 3
		SUM(status = 'unstable'),  -- 4
		SUM(status = 'down')       -- 5
	FROM results
	WHERE checked_at >= ? -- 1
	GROUP BY 1, 2;`

func (s *Store) hourly(since time.Time) (map[string]map[string]hourCount, error) {
	rows, err := s.db.Query(
		sqlHourly,
		since.UTC().Format(time.RFC3339), // 1
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	agg := make(map[string]map[string]hourCount)
	for rows.Next() {
		var service, hour string
		var c hourCount
		err = rows.Scan(
			&service,    // 1
			&hour,       // 2
			&c.Up,       // 3
			&c.Unstable, // 4
			&c.Down,     // 5
		)
		if err != nil {
			return nil, err
		}
		if agg[service] == nil {
			agg[service] = make(map[string]hourCount)
		}
		agg[service][hour] = c
	}
	return agg, rows.Err()
}

func (s *Store) prune(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM results WHERE checked_at < ?;`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
