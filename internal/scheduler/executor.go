package scheduler

import (
	"context"
	"fmt"

	"task110-gosched/internal/model"
)

// Executor runs a single task. The default implementation is deterministic so the
// engine can be tested and so failure paths are reproducible.
type Executor interface {
	Execute(ctx context.Context, task *model.Task, run *model.Run) (output string, err error)
}

// DefaultExecutor maps each TaskKind to a deterministic outcome.
type DefaultExecutor struct{}

// Execute returns (output, nil) for successful kinds and (output, err) for
// KindFail. It performs no real I/O, keeping runs fast and reproducible.
func (DefaultExecutor) Execute(_ context.Context, task *model.Task, _ *model.Run) (string, error) {
	switch task.Kind {
	case model.KindNoop:
		return "ok", nil
	case model.KindEcho:
		return task.Payload, nil
	case model.KindHTTP:
		return fmt.Sprintf("http POST dispatched; payload=%q", task.Payload), nil
	case model.KindFail:
		return "", fmt.Errorf("simulated failure: kind=fail")
	default:
		return "", fmt.Errorf("unknown task kind %q", task.Kind)
	}
}
