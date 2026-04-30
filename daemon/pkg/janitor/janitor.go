package janitor

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Janitor interface {
	Add(func(context.Context) error)
	Cleanup() error
}

func NewJanitor(cleanupTimeout time.Duration) Janitor {
	return &janitor{
		callbacks: make([]func(context.Context) error, 0),
		timeout:   cleanupTimeout,
	}
}

type janitor struct {
	mu        sync.Mutex
	callbacks []func(context.Context) error
	timeout   time.Duration
}

func (j *janitor) Add(cb func(context.Context) error) {
	if cb == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	j.callbacks = append(j.callbacks, cb)
}

func (j *janitor) Cleanup() error {
	j.mu.Lock()

	// Snapshot callbacks so Cleanup is stable even if Add()
	// is called concurrently.
	callbacks := make([]func(context.Context) error, len(j.callbacks))
	copy(callbacks, j.callbacks)

	j.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), j.timeout)
	defer cancel()

	var errs []error

	// Run in reverse order (stack semantics).
	for i := len(callbacks) - 1; i >= 0; i-- {
		if err := callbacks[i](ctx); err != nil {
			errs = append(errs, err)
		}

		// Stop early if timeout/cancellation hit.
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
	}

	return errors.Join(errs...)
}
