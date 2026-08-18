// Package store is the SQLite persistence layer for the scheduler engine. It
// owns the schema and the low-level transaction primitives; the scheduler and
// HTTP API layer business rules (due detection, run lifecycle, retries) on top.
//
// All write operations are wrapped in BEGIN IMMEDIATE transactions so concurrent
// writers (the scheduler dispatching while a run is being retried) serialize at
// the database level and exactly one transition wins.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; CGO_ENABLED=0 compatible
)

// Store wraps a *sql.DB connection to the scheduler database.
type Store struct {
	db *sql.DB
}

// Open creates or opens the SQLite file at path, applies the schema, and tunes
// pragmatic options for durability and concurrency:
//   - journal_mode=WAL: readers don't block a single writer;
//   - busy_timeout: a concurrent writer waits rather than failing fast;
//   - _txlock=immediate: every BEGIN is BEGIN IMMEDIATE, serializing writers.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_txlock=immediate",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite effectively serializes writes; one connection is enough and avoids
	// "database is locked" from interleaved write transactions on extra conns.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// BeginTx starts an IMMEDIATE transaction. Callers must Commit or Rollback.
func (s *Store) BeginTx() (*sql.Tx, error) { return s.db.Begin() }

// DB exposes the underlying handle for read-only queries that don't need a tx.
func (s *Store) DB() *sql.DB { return s.db }

// newID returns a random hex identifier with the given prefix.
func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail; fall back to a timestamp-based id.
		return fmt.Sprintf("%s-fallback", prefix)
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// NewID is the exported entry point for generating random identifiers.
func NewID(prefix string) string { return newID(prefix) }
