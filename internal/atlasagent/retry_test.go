package atlasagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrierSucceedsFirstAttemptWithoutSleeping(t *testing.T) {
	var slept []time.Duration
	retrier := Retrier{Initial: time.Second, Max: time.Minute, Sleep: func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}

	attempts := 0
	err := retrier.Do(context.Background(), func(err error, delay time.Duration) {}, func() error {
		attempts++
		return nil
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if attempts != 1 || len(slept) != 0 {
		t.Fatalf("attempts = %d, sleeps = %v, want 1 attempt and no sleep", attempts, slept)
	}
}

func TestRetrierBacksOffExponentiallyUntilSuccess(t *testing.T) {
	var slept []time.Duration
	retrier := Retrier{Initial: time.Second, Max: time.Minute, Sleep: func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}

	attempts := 0
	err := retrier.Do(context.Background(), func(err error, delay time.Duration) {}, func() error {
		attempts++
		if attempts < 4 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4", attempts)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(slept) != len(want) {
		t.Fatalf("slept %v, want %v", slept, want)
	}
	for i := range want {
		if slept[i] != want[i] {
			t.Fatalf("slept %v, want %v", slept, want)
		}
	}
}

func TestRetrierCapsBackoff(t *testing.T) {
	var slept []time.Duration
	retrier := Retrier{Initial: time.Second, Max: 4 * time.Second, Sleep: func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}

	attempts := 0
	err := retrier.Do(context.Background(), func(err error, delay time.Duration) {}, func() error {
		attempts++
		if attempts < 6 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second, 4 * time.Second}
	if len(slept) != len(want) {
		t.Fatalf("slept %v, want %v", slept, want)
	}
	for i := range want {
		if slept[i] != want[i] {
			t.Fatalf("slept %v, want %v", slept, want)
		}
	}
}

func TestRetrierReturnsPermanentErrorsImmediately(t *testing.T) {
	var slept []time.Duration
	retrier := Retrier{Initial: time.Second, Max: time.Minute, Sleep: func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}}

	permanent := &permanentError{err: errors.New("no retrying this")}
	attempts := 0
	err := retrier.Do(context.Background(), func(err error, delay time.Duration) {}, func() error {
		attempts++
		return permanent
	})
	if !errors.Is(err, permanent) {
		t.Fatalf("err = %v, want the permanent error itself", err)
	}
	if attempts != 1 || len(slept) != 0 {
		t.Fatalf("attempts = %d, sleeps = %v, want 1 attempt and no sleep", attempts, slept)
	}
}

func TestRetrierStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	retrier := Retrier{Initial: time.Second, Max: time.Minute, Sleep: func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}}

	lastAttempt := errors.New("transient")
	attempts := 0
	err := retrier.Do(ctx, func(err error, delay time.Duration) {}, func() error {
		attempts++
		return lastAttempt
	})
	if !errors.Is(err, lastAttempt) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the last attempt joined with the cancellation", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetrierReportsRetryDelays(t *testing.T) {
	var observed []time.Duration
	retrier := Retrier{Initial: time.Second, Max: time.Minute, Sleep: func(ctx context.Context, d time.Duration) error {
		return nil
	}}

	attempts := 0
	_ = retrier.Do(context.Background(), func(err error, delay time.Duration) {
		observed = append(observed, delay)
	}, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if len(observed) != 2 || observed[0] != time.Second || observed[1] != 2*time.Second {
		t.Fatalf("observed delays = %v, want [1s 2s]", observed)
	}
}
