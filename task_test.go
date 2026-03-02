package doubaospeech

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTaskWaitPollsImmediatelyBeforeInterval(t *testing.T) {
	var calls int
	task := NewTask[string](
		"task-1",
		func(_ context.Context, _ string) (TaskStatus, *string, error) {
			calls++
			return TaskStatusPending, nil, nil
		},
		500*time.Millisecond,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := task.Wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait error = %v, want deadline exceeded", err)
	}
	if calls != 1 {
		t.Fatalf("poll calls = %d, want 1", calls)
	}
}

func TestTaskWaitUnknownStatusReturnsError(t *testing.T) {
	task := NewTask[string](
		"task-2",
		func(_ context.Context, _ string) (TaskStatus, *string, error) {
			return "mystery", nil, nil
		},
		10*time.Millisecond,
	)

	_, err := task.Wait(context.Background())
	if err == nil {
		t.Fatalf("expected unknown status error")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeServerError {
		t.Fatalf("error code = %d, want %d", apiErr.Code, CodeServerError)
	}
	if !strings.Contains(apiErr.Message, "unknown task status") {
		t.Fatalf("error message = %q, want contains %q", apiErr.Message, "unknown task status")
	}
}

func TestNormalizeTaskStatusSupportsAliasesAndCodes(t *testing.T) {
	tests := []struct {
		name string
		raw  TaskStatus
		want TaskStatus
	}{
		{name: "numeric processing", raw: "1", want: TaskStatusProcessing},
		{name: "numeric success active", raw: "4", want: TaskStatusSuccess},
		{name: "training alias", raw: "training", want: TaskStatusProcessing},
		{name: "active alias", raw: "active", want: TaskStatusSuccess},
		{name: "cancelled alias", raw: "canceled", want: TaskStatusCancelled},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeTaskStatus(tc.raw)
			if got != tc.want {
				t.Fatalf("normalizeTaskStatus(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
