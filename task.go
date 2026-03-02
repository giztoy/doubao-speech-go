package doubaospeech

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const defaultTaskPollInterval = 2 * time.Second

// TaskPoller polls current task status and optional result.
type TaskPoller[T any] func(ctx context.Context, taskID string) (status TaskStatus, result *T, err error)

// TaskStatusMapper normalizes raw task status values.
type TaskStatusMapper func(status TaskStatus) TaskStatus

// TaskFailureMapper converts terminal failure status to a concrete error.
type TaskFailureMapper[T any] func(status TaskStatus, result *T) error

// Task represents an asynchronous task.
type Task[T any] struct {
	ID       string
	poller   TaskPoller[T]
	interval time.Duration

	statusMapper  TaskStatusMapper
	failureMapper TaskFailureMapper[T]
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

		statusMapper:  defaultTaskStatusMapper,
		failureMapper: defaultTaskFailureMapper[T],
	}
}

// SetStatusMapper customizes task status normalization.
func (t *Task[T]) SetStatusMapper(mapper TaskStatusMapper) *Task[T] {
	if t == nil || mapper == nil {
		return t
	}
	t.statusMapper = mapper
	return t
}

// SetFailureMapper customizes terminal task failure error mapping.
func (t *Task[T]) SetFailureMapper(mapper TaskFailureMapper[T]) *Task[T] {
	if t == nil || mapper == nil {
		return t
	}
	t.failureMapper = mapper
	return t
}

// Wait blocks until task completion.
func (t *Task[T]) Wait(ctx context.Context) (*T, error) {
	if t == nil {
		return nil, newAPIError(CodeParamError, "task is nil")
	}
	if t.poller == nil {
		return nil, newAPIError(CodeParamError, "task poller is nil")
	}
	if t.statusMapper == nil {
		t.statusMapper = defaultTaskStatusMapper
	}
	if t.failureMapper == nil {
		t.failureMapper = defaultTaskFailureMapper[T]
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		status, result, err := t.poller(ctx, t.ID)
		if err != nil {
			return nil, err
		}

		normalized := t.statusMapper(status)
		switch normalized {
		case TaskStatusSuccess:
			return result, nil
		case TaskStatusFailed, TaskStatusCancelled:
			return nil, t.failureMapper(normalized, result)
		case TaskStatusPending, TaskStatusProcessing:
			// continue polling
		default:
			return nil, newAPIError(CodeServerError, fmt.Sprintf("unknown task status: %q", status))
		}

		timer := time.NewTimer(t.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func defaultTaskFailureMapper[T any](status TaskStatus, _ *T) error {
	switch status {
	case TaskStatusFailed:
		return newAPIError(CodeServerError, "task failed")
	case TaskStatusCancelled:
		return newAPIError(CodeServerError, "task cancelled")
	default:
		return newAPIError(CodeServerError, "task terminated")
	}
}

func defaultTaskStatusMapper(status TaskStatus) TaskStatus {
	return normalizeTaskStatus(status)
}

func normalizeTaskStatus(status TaskStatus) TaskStatus {
	s := strings.ToLower(strings.TrimSpace(string(status)))
	if s == "" {
		return ""
	}

	if code, err := strconv.Atoi(s); err == nil {
		return mapTaskStatusCode(code)
	}

	switch s {
	case string(TaskStatusPending), "queued", "created", "submitted", "not_found", "notfound":
		return TaskStatusPending
	case string(TaskStatusProcessing), "running", "in_progress", "training":
		return TaskStatusProcessing
	case string(TaskStatusSuccess), "done", "completed", "succeeded", "ready", "active":
		return TaskStatusSuccess
	case string(TaskStatusFailed), "error":
		return TaskStatusFailed
	case string(TaskStatusCancelled), "canceled", "aborted", "reclaimed":
		return TaskStatusCancelled
	default:
		return TaskStatus(status)
	}
}

func mapTaskStatusCode(code int) TaskStatus {
	switch code {
	case 0:
		return TaskStatusPending
	case 1:
		return TaskStatusProcessing
	case 2, 4:
		return TaskStatusSuccess
	case 3:
		return TaskStatusFailed
	case 5:
		return TaskStatusCancelled
	default:
		return ""
	}
}
