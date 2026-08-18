package store

// schema is applied on every Open; SQLite's IF NOT EXISTS makes it idempotent so
// a fresh file, a reused file and a post-crash file all converge to the same
// shape. tasks/runs/tags carry the durable state; stats_daily backs the
// per-day aggregate used by /api/v1/stats/daily.
const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id              TEXT PRIMARY KEY,
	name            TEXT    NOT NULL,
	kind            TEXT    NOT NULL,
	spec_type       TEXT    NOT NULL,
	cron_expr       TEXT    NOT NULL DEFAULT '',
	delay_seconds   INTEGER NOT NULL DEFAULT 0,
	payload         TEXT    NOT NULL DEFAULT '',
	enabled         INTEGER NOT NULL DEFAULT 1,
	paused          INTEGER NOT NULL DEFAULT 0,
	tags            TEXT    NOT NULL DEFAULT '',
	max_retries     INTEGER NOT NULL DEFAULT 0,
	timeout_seconds INTEGER NOT NULL DEFAULT 0,
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL,
	next_run_at     INTEGER NOT NULL DEFAULT 0,
	last_run_at     INTEGER NOT NULL DEFAULT 0,
	version         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_tasks_next    ON tasks(next_run_at);
CREATE INDEX IF NOT EXISTS idx_tasks_state   ON tasks(enabled, paused);

CREATE TABLE IF NOT EXISTS runs (
	id           TEXT    PRIMARY KEY,
	task_id      TEXT    NOT NULL,
	task_name    TEXT    NOT NULL DEFAULT '',
	attempt      INTEGER NOT NULL DEFAULT 1,
	status       TEXT    NOT NULL,
	scheduled_at INTEGER NOT NULL,
	started_at   INTEGER NOT NULL DEFAULT 0,
	finished_at  INTEGER NOT NULL DEFAULT 0,
	triggered_by TEXT    NOT NULL DEFAULT '',
	error        TEXT    NOT NULL DEFAULT '',
	output       TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_runs_task   ON runs(task_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

CREATE TABLE IF NOT EXISTS tags (
	name       TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS stats_daily (
	day       TEXT PRIMARY KEY,
	succeeded INTEGER NOT NULL DEFAULT 0,
	failed    INTEGER NOT NULL DEFAULT 0,
	running   INTEGER NOT NULL DEFAULT 0
);
`
