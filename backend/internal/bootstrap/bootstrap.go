// Package bootstrap provides application lifecycle helpers.
package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
)

// App manages application lifecycle with graceful shutdown support.
type App struct {
	mu    sync.Mutex
	hooks []func(ctx context.Context) error
}

// New creates a new App.
func New() *App {
	return &App{}
}

// AddShutdownHook registers a function to call during graceful shutdown.
// Hooks run in reverse order (LIFO). Thread-safe.
func (a *App) AddShutdownHook(fn func(ctx context.Context) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hooks = append(a.hooks, fn)
}

// Run sets up signal handling and executes the run function.
// On OS interrupt, it calls registered shutdown hooks in LIFO order.
// If run returns an error before a signal, that error is returned.
func (a *App) Run(ctx context.Context, run func(ctx context.Context) error) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := run(ctx); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		return a.shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (a *App) shutdown(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var errs []error
	for i := len(a.hooks) - 1; i >= 0; i-- {
		if err := a.hooks[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
