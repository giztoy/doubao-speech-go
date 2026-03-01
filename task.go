package doubaospeech

import (
	"context"
	"time"
)

const defaultTaskPollInterval = 2 * time.Second

// TaskPoller 轮询函数，返回当前状态与可选结果。
type TaskPoller[T any] func(ctx context.Context, taskID string) (status TaskStatus, result *T, err error)

// Task 表示异步任务。
type Task[T any] struct {
	ID       string
	poller   TaskPoller[T]
	interval time.Duration
}

// NewTask 创建异步任务句柄。
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

// Wait 阻塞轮询直到任务结束。
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
