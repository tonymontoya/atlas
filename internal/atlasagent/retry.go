package atlasagent

import (
	"context"
	"errors"
	"time"
)

// Retrier runs an attempt function with exponential backoff. Attempt
// errors classified permanent (IsPermanent) surface immediately; every
// other failure backs off — Initial doubling per retry, capped at Max —
// until the attempt succeeds or the context ends. Sleep is injectable
// for tests; production callers leave it nil for the timer-based
// default.
type Retrier struct {
	Initial time.Duration
	Max     time.Duration
	Sleep   func(ctx context.Context, delay time.Duration) error
}

// Do runs attempt until it succeeds, fails permanently, or ctx ends.
// onRetry observes every failed attempt with the delay that follows
// it. A cancelled context returns the last attempt's error joined with
// the context error.
func (r *Retrier) Do(ctx context.Context, onRetry func(err error, delay time.Duration), attempt func() error) error {
	delay := r.Initial
	for {
		err := attempt()
		if err == nil {
			return nil
		}
		if IsPermanent(err) {
			return err
		}
		if r.Max > 0 && delay > r.Max {
			delay = r.Max
		}
		if onRetry != nil {
			onRetry(err, delay)
		}
		sleepErr := r.sleepFunc()(ctx, delay)
		if sleepErr != nil {
			return errors.Join(err, sleepErr)
		}
		if delay < r.Max {
			delay *= 2
		}
	}
}

func (r *Retrier) sleepFunc() func(context.Context, time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep
	}
	return sleepContext
}

// sleepContext waits for the delay or the context's end, whichever
// comes first.
func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
