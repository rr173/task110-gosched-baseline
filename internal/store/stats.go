package store

import (
	"database/sql"

	"task110-gosched/internal/model"
)

// RecordRunOutcome upserts the per-day counters for the run's scheduled day.
// succeeded / failed increment on terminal outcomes; running increments while
// the run is in-flight. The first insert seeds the target column at 1, and a
// conflicted row increments it, so both paths count the run.
func (s *Store) RecordRunOutcome(tx *sql.Tx, day string, status model.RunStatus) error {
	col := ""
	switch status {
	case model.RunSucceeded:
		col = "succeeded"
	case model.RunFailed, model.RunOrphaned:
		col = "failed"
	case model.RunRunning:
		col = "running"
	default:
		return nil // pending does not affect aggregates yet
	}
	q := `INSERT INTO stats_daily(day, ` + col + `) VALUES(?,1)
		  ON CONFLICT(day) DO UPDATE SET ` + col + `=` + col + `+1`
	_, err := tx.Exec(q, day)
	return err
}

// DailyStats returns per-day aggregates within [fromDay, toDay] inclusive (days
// are "YYYY-MM-DD" strings, compared lexicographically).
func (s *Store) DailyStats(tx *sql.Tx, fromDay, toDay string) ([]model.DailyStat, error) {
	rows, err := tx.Query(
		`SELECT day, succeeded, failed, running FROM stats_daily WHERE day>=? AND day<=? ORDER BY day`,
		fromDay, toDay,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DailyStat
	for rows.Next() {
		var d model.DailyStat
		if err := rows.Scan(&d.Day, &d.Succeeded, &d.Failed, &d.Running); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// OverallStats aggregates task and run counts.
func (s *Store) OverallStats(tx *sql.Tx) (*model.Stats, error) {
	var st model.Stats
	if err := tx.QueryRow(
		`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN enabled=1 AND paused=0 THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN paused=1 THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN enabled=0 THEN 1 ELSE 0 END),0)
		 FROM tasks`,
	).Scan(&st.TasksTotal, &st.TasksActive, &st.TasksPaused, &st.TasksDisabled); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(
		`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status='succeeded' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='failed' OR status='orphaned' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='orphaned' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='running' THEN 1 ELSE 0 END),0)
		 FROM runs`,
	).Scan(&st.RunsTotal, &st.RunsSucceeded, &st.RunsFailed, &st.RunsOrphaned, &st.RunsRunning); err != nil {
		return nil, err
	}
	return &st, nil
}
