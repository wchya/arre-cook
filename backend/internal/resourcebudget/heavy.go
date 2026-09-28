// Package resourcebudget admits memory-heavy work before input allocation.
// Image decode and local ASR share one reservation on the small mixed-use host.
package resourcebudget

import (
	"context"
	"errors"
	"sync"
)

var slot = make(chan struct{}, 1)
var ErrBusy = errors.New("heavy task capacity exhausted")

func Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case slot <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-slot }) }, nil
	default:
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
