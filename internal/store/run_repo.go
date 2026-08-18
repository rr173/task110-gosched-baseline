package store

import (
	"database/sql"
	"strings"

	"task110-gosched/internal/model"
)

func scanRunRow(s scanner) (*model.Run, error) {
	var (
		r           model.Run
		taskName    string
		startedAt   int64
		finishedAt  int64
		triggeredBy string
		errMsg      string
		output      string
	)
	err := s.Scan(
		&r.ID, &r.TaskID, &taskName, &r.Attempt, &r.Status,
		&r.ScheduledAt, &startedAt, &finishedAt, &triggeredBy, &errMsg, &output,
	)
	if err != nil {
		return nil, err
	}
	r.TaskName = taskName
	r.StartedAt = startedAt
	r.FinishedAt = finishedAt
	r.TriggeredBy = model.Trigger(triggeredBy)
	r.Error = errMsg
	r.Output = output
	return &r, nil
}

const runCols = `id,task_id,task_name,attempt,status,scheduled_at,started_at,finished_at,triggered_by,error,output`

// CreateRun inserts a run record.
func (s *Store) CreateRun(tx *sql.Tx, r *model.Run) error {
	_, err := tx.Exec(
		`INSERT INTO runs(id,task_id,task_name,attempt,status,scheduled_at,started_at,finished_at,triggered_by,error,output)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.TaskID, r.TaskName, r.Attempt, string(r.Status),
		r.ScheduledAt, r.StartedAt, r.FinishedAt, string(r.TriggeredBy), r.Error, r.Output,
	)
	return err
}

// GetRun loads a run by id. Returns (nil, nil) when absent.
func (s *Store) GetRun(tx *sql.Tx, id string) (*model.Run, error) {
	row := tx.QueryRow(`SELECT `+runCols+` FROM runs WHERE id=?`, id)
	r, err := scanRunRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

// RunFilter describes list filters for runs.
type RunFilter struct {
	TaskID string
	Status string
	Limit  int
	Offset int
}

// ListRuns returns runs matching the filter, newest first.
func (s *Store) ListRuns(tx *sql.Tx, f RunFilter) ([]model.Run, error) {
	var where []string
	var args []interface{}
	if f.TaskID != "" {
		where = append(where, "task_id=?")
		args = append(args, f.TaskID)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	q := `SELECT ` + runCols + ` FROM runs`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY scheduled_at DESC, id DESC"
	if f.Limit > 0 {
		q += " LIMIT ?"
		if f.Offset > 0 {
			args = append(args, f.Offset)
			q += " OFFSET ?"
			args = append(args, f.Limit)
		} else {
			args = append(args, f.Limit)
		}
	}
	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Run
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// UpdateRunStatus updates the lifecycle fields of a run atomically.
func (s *Store) UpdateRunStatus(tx *sql.Tx, id string, status model.RunStatus, startedAt, finishedAt int64, output, errMsg string) error {
	_, err := tx.Exec(
		`UPDATE runs SET status=?,started_at=?,finished_at=?,output=? WHERE id=?`,
		string(status), startedAt, finishedAt, output, id,
	)
	return err
}

// RetryRun creates a new run for the same task with attempt+1 and trigger=retry.
// The returned run has status pending and zero start/finish times.
func (s *Store) RetryRun(tx *sql.Tx, orig *model.Run, scheduledAt int64) (*model.Run, error) {
	r := &model.Run{
		ID:          newID("run"),
		TaskID:      orig.TaskID,
		TaskName:    orig.TaskName,
		Attempt:     orig.Attempt + 1,
		Status:      model.RunPending,
		ScheduledAt: scheduledAt,
		TriggeredBy: model.TriggerRetry,
	}
	if err := s.CreateRun(tx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// DeleteRun removes a single run record.
func (s *Store) DeleteRun(tx *sql.Tx, id string) error {
	_, err := tx.Exec(`DELETE FROM runs WHERE id=?`, id)
	return err
}

// OrphanedRuns returns runs left in "running" state (process died mid-flight).
func (s *Store) OrphanedRuns(tx *sql.Tx) ([]model.Run, error) {
	rows, err := tx.Query(`SELECT `+runCols+` FROM runs WHERE status=? ORDER BY scheduled_at ASC`, string(model.RunRunning))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Run
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}
