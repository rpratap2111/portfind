// Package history records what happened to processes on ports and detects
// "port fights": a port that portfind has had to kill repeatedly in a short
// window, which usually means something (nodemon, a supervisor, a service
// with auto-restart) keeps respawning the process.
//
// Storage is SQLite via modernc.org/sqlite, a pure-Go driver, so portfind
// needs no cgo and cross-compiles cleanly.
package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Port-fight detection defaults: this many portfind kills of the same port
// within the window triggers a hint.
const (
	FightWindow    = 15 * time.Minute
	FightThreshold = 3
)

const schemaVersion = 1

const schemaV1 = `
CREATE TABLE port_events (
	id                  INTEGER PRIMARY KEY,
	at_ms               INTEGER NOT NULL, -- unix milliseconds
	port                INTEGER NOT NULL,
	pid                 INTEGER NOT NULL,
	process             TEXT    NOT NULL,
	project             TEXT    NOT NULL DEFAULT '',
	killed_via_portfind INTEGER NOT NULL  -- 1: portfind killed it; 0: it went away some other way
);
CREATE INDEX port_events_port_at ON port_events (port, at_ms);
CREATE INDEX port_events_at ON port_events (at_ms);
`

// Event is one process leaving one port.
type Event struct {
	At                time.Time
	Port              int
	PID               int
	Process           string
	Project           string
	KilledViaPortfind bool
}

// Fight is a port that portfind killed at least FightThreshold times within
// the window. Process and Project describe the most recent kill.
type Fight struct {
	Port    int
	Kills   int
	Last    time.Time
	Process string
	Project string
}

// Store is an open history database. It is safe for concurrent use, and the
// TUI and tray app may share one file (WAL mode plus a busy timeout).
type Store struct {
	db *sql.DB
}

// DefaultPath is $PORTFIND_HISTORY_DB if set, otherwise
// %LocalAppData%\portfind\history.db on Windows (the equivalent user cache
// directory elsewhere).
func DefaultPath() (string, error) {
	if p := os.Getenv("PORTFIND_HISTORY_DB"); p != "" {
		return p, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user data directory: %w", err)
	}
	return filepath.Join(dir, "portfind", "history.db"), nil
}

// Open opens or creates the database at path and migrates its schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open history database %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite allows one writer; serialize in-process
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("history database %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	switch {
	case v == schemaVersion:
		return nil
	case v > schemaVersion:
		return fmt.Errorf("schema version %d is newer than this portfind supports (%d)", v, schemaVersion)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV1); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Record appends an event. A zero At means now.
func (s *Store) Record(e Event) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO port_events (at_ms, port, pid, process, project, killed_via_portfind) VALUES (?, ?, ?, ?, ?, ?)`,
		e.At.UnixMilli(), e.Port, e.PID, e.Process, e.Project, e.KilledViaPortfind)
	if err != nil {
		return fmt.Errorf("record history event for :%d: %w", e.Port, err)
	}
	return nil
}

// Recent returns up to limit events, newest first.
func (s *Store) Recent(limit int) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT at_ms, port, pid, process, project, killed_via_portfind
		   FROM port_events ORDER BY at_ms DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var atMS int64
		if err := rows.Scan(&atMS, &e.Port, &e.PID, &e.Process, &e.Project, &e.KilledViaPortfind); err != nil {
			return nil, fmt.Errorf("read history row: %w", err)
		}
		e.At = time.UnixMilli(atMS)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	return out, nil
}

// PortFights returns ports killed via portfind at least threshold times
// since the given time, most recent first.
func (s *Store) PortFights(since time.Time, threshold int) ([]Fight, error) {
	rows, err := s.db.Query(`
		WITH kills AS (
			SELECT id, at_ms, port, pid, process, project
			  FROM port_events
			 WHERE killed_via_portfind = 1 AND at_ms >= ?
		), fights AS (
			SELECT port, COUNT(*) AS n, MAX(id) AS last_id
			  FROM kills GROUP BY port HAVING COUNT(*) >= ?
		)
		SELECT f.port, f.n, k.at_ms, k.process, k.project
		  FROM fights f JOIN kills k ON k.id = f.last_id
		 ORDER BY k.at_ms DESC`,
		since.UnixMilli(), threshold)
	if err != nil {
		return nil, fmt.Errorf("query port fights: %w", err)
	}
	defer rows.Close()

	var out []Fight
	for rows.Next() {
		var f Fight
		var atMS int64
		if err := rows.Scan(&f.Port, &f.Kills, &atMS, &f.Process, &f.Project); err != nil {
			return nil, fmt.Errorf("read port fight: %w", err)
		}
		f.Last = time.UnixMilli(atMS)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read port fights: %w", err)
	}
	return out, nil
}
