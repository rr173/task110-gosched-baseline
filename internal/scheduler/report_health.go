package scheduler

func (r SchedulerReport) Healthy() bool {
	if r.Tasks.Due < 0 || r.Tasks.Enabled > r.Tasks.Total || r.Tasks.Paused > r.Tasks.Total {
		return false
	}
	if r.Durable == nil {
		return false
	}
	return r.Durable.RunsSucceeded+r.Durable.RunsFailed+r.Durable.RunsOrphaned >= 0
}

func (r SchedulerReport) Summary() map[string]any {
	return map[string]any{"total_tasks": r.Tasks.Total, "enabled_tasks": r.Tasks.Enabled, "paused_tasks": r.Tasks.Paused, "due_tasks": r.Tasks.Due, "healthy": r.Healthy()}
}
