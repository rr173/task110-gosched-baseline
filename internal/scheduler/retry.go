package scheduler

import "task110-gosched/internal/model"

// CanRetry reports whether another attempt is allowed for a failed run given the
// task's max_retries budget. attempt is 1-based, so a run with attempt N may be
// retried while N <= max_retries.
func CanRetry(task *model.Task, run *model.Run) bool {
	return run.Attempt <= task.MaxRetries
}

// BackoffMillis returns the suggested backoff in milliseconds before the next
// attempt: 1s, 2s, 4s, ... capped at 60s. It is purely advisory; the engine
// executes retries eagerly on recovery/manual retry.
func BackoffMillis(attempt int) int64 {
	b := int64(1000) << uint(attempt-1) // 1s, 2s, 4s, 8s ...
	if b > 60000 {
		b = 60000
	}
	return b
}
