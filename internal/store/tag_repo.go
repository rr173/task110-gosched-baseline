package store

import (
	"database/sql"

	"task110-gosched/internal/model"
)

// CreateTag inserts a tag if absent. It is idempotent.
func (s *Store) CreateTag(tx *sql.Tx, name string, createdAt int64) error {
	_, err := tx.Exec(
		`INSERT INTO tags(name, created_at) VALUES(?,?) ON CONFLICT(name) DO NOTHING`,
		name, createdAt,
	)
	return err
}

// GetTag loads a tag by name. Returns (nil, nil) when absent.
func (s *Store) GetTag(tx *sql.Tx, name string) (*model.Tag, error) {
	row := tx.QueryRow(`SELECT name, created_at FROM tags WHERE name=?`, name)
	var t model.Tag
	err := row.Scan(&t.Name, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &t, err
}

// ListTags returns all tags ordered by name.
func (s *Store) ListTags(tx *sql.Tx) ([]model.Tag, error) {
	rows, err := tx.Query(`SELECT name, created_at FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TasksByTag returns tasks carrying the given tag (newest first).
func (s *Store) TasksByTag(tx *sql.Tx, tag string) ([]model.Task, error) {
	return s.ListTasks(tx, TaskFilter{Tag: tag})
}
