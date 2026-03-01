package doubaospeech

import (
	"context"
	"time"
)

const defaultTaskPollInterval = 2 * time.Second

// TaskPoller polls current task status and optional result.
type TaskPoller[T any] func(ctx context.Context, taskID string) (status TaskStatus, result *T, err error)

// Task represents an asynchronous task.
type Task[T any] struct {
	ID       string
	poller   TaskPoller[T]
	interval time.Duration
}

// NewTask creates an asynchronous task handle.
func NewTask[T any](id string, poller TaskPoller[T], interval time.Duration) *Task[T] {
	if interval <= 0 {
		interval = defaultTaskPollInterval
	}
	return &Task[T]{
		ID:       id,
		poller:   poller,
		interval: interval,
	}
}

// Wait blocks until task completion.
func (t *Task[T]) Wait(ctx context.Context) (*T, error) {
	if t == nil {
		return nil, newAPIError(CodeParamError, "task is nil")
	}
	if t.poller == nil {
		return nil, newAPIError(CodeParamError, "task poller is nil")
	}

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, result, err := t.poller(ctx, t.ID)
			if err != nil {
				return nil, err
			}

			switch status {
			case TaskStatusSuccess:
				return result, nil
			case TaskStatusFailed:
				return nil, newAPIError(CodeServerError, "task failed")
			case TaskStatusCancelled:
				return nil, newAPIError(CodeServerError, "task cancelled")
			case TaskStatusPending, TaskStatusProcessing:
				// continue polling
			default:
				return nil, newAPIError(CodeServerError, "unknown task status")
			}
		}
	}
}
