package store

import (
	"database/sql"
	"strings"

	"task110-gosched/internal/model"
)

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTaskRow(s scanner) (*model.Task, error) {
	var (
		t            model.Task
		cronExpr     string
		tagsCSV      string
		enabled      int
		paused       int
		delaySeconds int
		maxRetries   int
		timeoutSecs  int
	)
	err := s.Scan(
		&t.ID, &t.Name, &t.Kind, &t.SpecType, &cronExpr, &delaySeconds,
		&t.Payload, &enabled, &paused, &tagsCSV, &maxRetries, &timeoutSecs,
		&t.CreatedAt, &t.UpdatedAt, &t.NextRunAt, &t.LastRunAt, &t.Version,
	)
	if err != nil {
		return nil, err
	}
	t.CronExpr = cronExpr
	t.DelaySeconds = int64(delaySeconds)
	t.Enabled = enabled != 0
	t.Paused = paused != 0
	t.MaxRetries = maxRetries
	t.TimeoutSeconds = timeoutSecs
	if tagsCSV != "" {
		t.Tags = strings.Split(tagsCSV, ",")
	}
	return &t, nil
}

const taskCols = `id,name,kind,spec_type,cron_expr,delay_seconds,payload,enabled,paused,tags,max_retries,timeout_seconds,created_at,updated_at,next_run_at,last_run_at,version`

// CreateTask inserts a task. The caller is responsible for assigning IDs and
// timestamps.
func (s *Store) CreateTask(tx *sql.Tx, t *model.Task) error {
	_, err := tx.Exec(
		`INSERT INTO tasks(id,name,kind,spec_type,cron_expr,delay_seconds,payload,enabled,paused,tags,max_retries,timeout_seconds,created_at,updated_at,next_run_at,last_run_at,version)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, string(t.Kind), string(t.SpecType), t.CronExpr, t.DelaySeconds,
		t.Payload, boolToInt(t.Enabled), boolToInt(t.Paused), model.TagsCSV(t.Tags),
		t.MaxRetries, t.TimeoutSeconds, t.CreatedAt, t.UpdatedAt, t.NextRunAt,
		t.LastRunAt, t.Version,
	)
	return err
}

// GetTask loads a task by id. Returns (nil, nil) when absent.
func (s *Store) GetTask(tx *sql.Tx, id string) (*model.Task, error) {
	row := tx.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id=?`, id)
	t, err := scanTaskRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// UpdateTask replaces all mutable columns of the task identified by id.
func (s *Store) UpdateTask(tx *sql.Tx, t *model.Task) error {
	_, err := tx.Exec(
		`UPDATE tasks SET name=?,kind=?,spec_type=?,cron_expr=?,delay_seconds=?,payload=?,enabled=?,paused=?,tags=?,max_retries=?,timeout_seconds=?,updated_at=?,next_run_at=?,last_run_at=?,version=? WHERE id=?`,
		t.Name, string(t.Kind), string(t.SpecType), t.CronExpr, t.DelaySeconds,
		t.Payload, boolToInt(t.Enabled), boolToInt(t.Paused), model.TagsCSV(t.Tags),
		t.MaxRetries, t.TimeoutSeconds, t.UpdatedAt, t.NextRunAt, t.LastRunAt,
		t.Version, t.ID,
	)
	return err
}

// DeleteTask removes a task and its runs (manual cascade, no FK enforced).
func (s *Store) DeleteTask(tx *sql.Tx, id string) error {
	if _, err := tx.Exec(`DELETE FROM runs WHERE task_id=?`, id); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

// TaskFilter describes list filters.
type TaskFilter struct {
	Tag     string
	Status  string // "", "active", "paused", "disabled"
	Enabled *bool
	Paused  *bool
	Limit   int
	Offset  int
}

// ListTasks returns tasks matching the filter, ordered by created_at desc.
func (s *Store) ListTasks(tx *sql.Tx, f TaskFilter) ([]model.Task, error) {
	var where []string
	var args []interface{}
	if f.Enabled != nil {
		where = append(where, "enabled=?")
		args = append(args, boolToInt(*f.Enabled))
	}
	if f.Paused != nil {
		where = append(where, "paused=?")
		args = append(args, boolToInt(*f.Paused))
	}
	switch f.Status {
	case "active":
		where = append(where, "enabled=1 AND paused=0")
	case "paused":
		where = append(where, "paused=1")
	case "disabled":
		where = append(where, "enabled=0")
	}
	if f.Tag != "" {
		// tags is a CSV; match as a substring with comma boundaries.
		where = append(where, "(tags=? OR tags LIKE ? OR tags LIKE ? OR tags LIKE ?)")
		args = append(args, f.Tag, f.Tag+",%", "%,"+f.Tag, "%,"+f.Tag+",%")
	}
	q := `SELECT ` + taskCols + ` FROM tasks`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += " OFFSET ?"
			args = append(args, f.Offset)
		}
	}
	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// DueTasks returns enabled, non-paused tasks whose next_run_at has arrived.
func (s *Store) DueTasks(tx *sql.Tx, nowMs int64) ([]model.Task, error) {
	rows, err := tx.Query(
		`SELECT `+taskCols+` FROM tasks WHERE enabled=1 AND paused=0 AND next_run_at>0 AND next_run_at<=? ORDER BY next_run_at ASC`,
		nowMs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// SetEnabled flips the enabled flag and bumps updated_at.
func (s *Store) SetEnabled(tx *sql.Tx, id string, enabled bool, updatedAt int64) error {
	_, err := tx.Exec(`UPDATE tasks SET enabled=?,updated_at=? WHERE id=?`, boolToInt(enabled), updatedAt, id)
	return err
}

// SetPaused flips the paused flag and bumps updated_at.
func (s *Store) SetPaused(tx *sql.Tx, id string, paused bool, updatedAt int64) error {
	_, err := tx.Exec(`UPDATE tasks SET paused=?,updated_at=? WHERE id=?`, boolToInt(paused), updatedAt, id)
	return err
}

// SetNextRun updates next_run_at and last_run_at for a task (scheduler bookkeeping).
func (s *Store) SetNextRun(tx *sql.Tx, id string, nextRunAt, lastRunAt, updatedAt int64) error {
	_, err := tx.Exec(
		`UPDATE tasks SET next_run_at=?,last_run_at=?,updated_at=? WHERE id=?`,
		nextRunAt, lastRunAt, updatedAt, id,
	)
	return err
}

// SetLastRun updates only last_run_at for a task.
func (s *Store) SetLastRun(tx *sql.Tx, id string, lastRunAt int64) error {
	_, err := tx.Exec(`UPDATE tasks SET last_run_at=? WHERE id=?`, lastRunAt, id)
	return err
}

// BulkPauseByTag pauses every active task carrying the tag. Returns the number
// of tasks paused.
func (s *Store) BulkPauseByTag(tx *sql.Tx, tag string, updatedAt int64) (int64, error) {
	res, err := tx.Exec(
		`UPDATE tasks SET paused=1,updated_at=? WHERE enabled=1 AND paused=0 AND (tags=? OR tags LIKE ? OR tags LIKE ? OR tags LIKE ?)`,
		updatedAt, tag, tag+",%", "%,"+tag+"x", "%,"+tag+",%",
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
