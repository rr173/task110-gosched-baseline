// Package scheduler drives the task engine: it creates/persists tasks, scans for
// due tasks on a ticker, executes them, records outcomes, advances cron
// schedules, and — on startup — recovers runs that were "running" when the
// process died (marks them orphaned and re-enqueues retries within budget).
package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"task110-gosched/internal/clock"
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
)

// Errors returned by the scheduler API surface.
var (
	ErrTaskNotFound  = errors.New("task not found")
	ErrRunNotFound   = errors.New("run not found")
	ErrNoRetryBudget = errors.New("no retry budget remaining")
)

// Scheduler is the engine. It is safe for concurrent use: every mutation goes
// through a BEGIN IMMEDIATE transaction in the store.
type Scheduler struct {
	store   *store.Store
	clock   clock.Clock
	exec    Executor
	metrics *metrics.Metrics
	tick    time.Duration
	stop    chan struct{}
	once    sync.Once
}

// New constructs a Scheduler. tick is the due-scan interval; a non-positive
// value defaults to one minute.
func New(st *store.Store, clk clock.Clock, exec Executor, m *metrics.Metrics, tick time.Duration) *Scheduler {
	if tick <= 0 {
		tick = time.Minute
	}
	return &Scheduler{
		store:   st,
		clock:   clk,
		exec:    exec,
		metrics: m,
		tick:    tick,
		stop:    make(chan struct{}),
	}
}

// ScheduleNew validates, computes the initial next-run time, ensures tags exist,
// and persists a new task.
func (s *Scheduler) ScheduleNew(task *model.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}
	now := s.clock.Now()
	nowMs := now.UnixMilli()
	task.CreatedAt = nowMs
	task.UpdatedAt = nowMs
	task.Version = 0
	switch task.SpecType {
	case model.SpecCron:
		n, err := task.ComputeNextCron(now)
		if err != nil {
			return err
		}
		task.NextRunAt = n.UnixMilli()
	case model.SpecDelayed:
		task.NextRunAt = nowMs + task.DelaySeconds*1000
	case model.SpecOnce:
		if task.NextRunAt == 0 {
			task.NextRunAt = nowMs
		}
	}

	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, tg := range task.Tags {
		if tg = cleanTag(tg); tg != "" {
			if err := s.store.CreateTag(tx, tg, nowMs); err != nil {
				return err
			}
		}
	}
	if err := s.store.CreateTask(tx, task); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.metrics.IncTasksCreated()
	return nil
}

// runOnce persists a run in "running" state, then executes it. The run is
// committed before execution so a crash leaves it in "running" for recovery.
func (s *Scheduler) runOnce(task *model.Task, attempt int, trigger model.Trigger, scheduledAt int64) error {
	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	run := &model.Run{
		ID:          store.NewID("run"),
		TaskID:      task.ID,
		TaskName:    task.Name,
		Attempt:     attempt,
		Status:      model.RunRunning,
		ScheduledAt: scheduledAt,
		StartedAt:   scheduledAt,
		TriggeredBy: trigger,
	}
	if err := s.store.CreateRun(tx, run); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.executeRun(task, run)
}

// executeRun runs the executor and records the outcome. It opens its own
// transaction so it can be called from dispatch, recovery and manual retry alike.
func (s *Scheduler) executeRun(task *model.Task, run *model.Run) error {
	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	output, execErr := s.exec.Execute(context.Background(), task, run)
	finished := s.clock.Now().UnixMilli()
	status := model.RunSucceeded
	errMsg := ""
	if execErr != nil {
		status = model.RunFailed
		errMsg = execErr.Error()
	}
	if err := s.store.UpdateRunStatus(tx, run.ID, status, run.StartedAt, finished, output, errMsg); err != nil {
		return err
	}
	day := time.UnixMilli(run.ScheduledAt).UTC().Format("2006-01-02")
	if err := s.store.RecordRunOutcome(tx, day, status); err != nil {
		return err
	}
	if err := s.store.SetLastRun(tx, task.ID, run.ScheduledAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.metrics.RecordRun(status, task.Kind)
	return nil
}

// dispatch executes one due scheduled task and advances its schedule.
func (s *Scheduler) dispatch(task *model.Task, nowMs int64) error {
	if err := s.runOnce(task, 1, model.TriggerSchedule, nowMs); err != nil {
		return err
	}
	next := int64(0)
	if task.SpecType == model.SpecCron {
		if n, err := task.ComputeNextCron(s.clock.Now()); err == nil {
			next = n.UnixMilli()
		}
	}
	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.store.SetNextRun(tx, task.ID, next, nowMs, s.clock.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// Tick scans for due tasks and dispatches them. Each task is independent; a
// failure on one does not block the others.
func (s *Scheduler) Tick() error {
	s.metrics.IncDispatches()
	nowMs := s.clock.Now().UnixMilli()
	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	due, err := s.store.DueTasks(tx, nowMs)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for i := range due {
		if err := s.dispatch(&due[i], nowMs); err != nil {
			// Log-and-continue: one bad task must not stall the scheduler.
			continue
		}
	}
	return nil
}

// Recover heals runs that were "running" when the process last stopped. Each is
// marked orphaned; if the task's retry budget allows, a fresh attempt is
// re-enqueued and executed. Returns the number of orphaned runs handled.
func (s *Scheduler) Recover() (int, error) {
	tx, err := s.store.BeginTx()
	if err != nil {
		return 0, err
	}
	orphans, err := s.store.OrphanedRuns(tx)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	type pending struct {
		task *model.Task
		run  *model.Run
	}
	var toExec []pending
	for _, o := range orphans {
		finished := s.clock.Now().UnixMilli()
		if err := s.store.UpdateRunStatus(tx, o.ID, model.RunOrphaned, o.StartedAt, finished, o.Output, "orphaned: process restarted mid-run"); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		task, err := s.store.GetTask(tx, o.TaskID)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		if task != nil && CanRetry(task, &o) {
			r, err := s.store.RetryRun(tx, &o, finished)
			if err != nil {
				_ = tx.Rollback()
				return 0, err
			}
			toExec = append(toExec, pending{task: task, run: r})
			s.metrics.IncRetried()
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	// Execute retries outside the recovery transaction.
	for _, p := range toExec {
		_ = s.executeRun(p.task, p.run)
	}
	return len(orphans), nil
}

// TriggerNow runs a task immediately, independent of its schedule.
func (s *Scheduler) TriggerNow(taskID string) error {
	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	task, err := s.store.GetTask(tx, taskID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_ = tx.Rollback()
	if task == nil {
		return ErrTaskNotFound
	}
	return s.runOnce(task, 1, model.TriggerManual, s.clock.Now().UnixMilli())
}

// RetryRunByID re-executes a failed run as a new attempt, if budget allows.
func (s *Scheduler) RetryRunByID(runID string) error {
	tx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	run, err := s.store.GetRun(tx, runID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if run == nil {
		_ = tx.Rollback()
		return ErrRunNotFound
	}
	task, err := s.store.GetTask(tx, run.TaskID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_ = tx.Rollback()
	if task == nil {
		return ErrTaskNotFound
	}
	if !CanRetry(task, run) {
		return ErrNoRetryBudget
	}
	rtx, err := s.store.BeginTx()
	if err != nil {
		return err
	}
	r, err := s.store.RetryRun(rtx, run, s.clock.Now().UnixMilli())
	if err != nil {
		_ = rtx.Rollback()
		return err
	}
	if err := rtx.Commit(); err != nil {
		return err
	}
	s.metrics.IncRetried()
	return s.executeRun(task, r)
}

// Start launches the background ticker.
func (s *Scheduler) Start() {
	go func() {
		ticker := time.NewTicker(s.tick)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				_ = s.Tick()
			}
		}
	}()
}

// Stop terminates the background ticker (idempotent).
func (s *Scheduler) Stop() {
	s.once.Do(func() { close(s.stop) })
}

func cleanTag(t string) string {
	for len(t) > 0 && (t[0] == ' ' || t[len(t)-1] == ' ') {
		t = t[1 : len(t)-1]
	}
	return t
}
