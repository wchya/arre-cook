package resourcebudget

import (
	"context"
	"errors"
	"testing"
)

func TestHeavyTasksShareOneReservation(t *testing.T) {
	release, err := Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal("overlapping memory-heavy task admitted")
	}
	release()
	release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled task admitted")
	}
	next, err := Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next()
}
