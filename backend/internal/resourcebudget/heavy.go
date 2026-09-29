// Package resourcebudget admits memory-heavy work before input allocation.
// Image decode and local ASR share one reservation on the small mixed-use host.
package resourcebudget

import (
	"context"
	"errors"
	"sync"
	"time"
)

var slot = make(chan struct{}, 1)
var ErrBusy = errors.New("heavy task capacity exhausted")

func Acquire(ctx context.Context) (func(), error) {
	return AcquireWait(ctx, 0)
}

// AcquireWait queues for up to wait before giving up, so short image decodes
// can line up behind each other (or a bounded ASR job) instead of failing.
func AcquireWait(ctx context.Context, wait time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release := func() func() {
		var once sync.Once
		return func() { once.Do(func() { <-slot }) }
	}
	select {
	case slot <- struct{}{}:
		return release(), nil
	default:
	}
	if wait <= 0 {
		return nil, ErrBusy
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case slot <- struct{}{}:
		return release(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrBusy
	}
}

// Drain waits for the single admitted job to release its resources after the
// server stops accepting new work and cancels any request past its deadline.
func Drain(ctx context.Context) error {
	select {
	case slot <- struct{}{}:
		<-slot
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
