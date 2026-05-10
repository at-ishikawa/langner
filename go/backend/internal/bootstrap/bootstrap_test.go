package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApp_Run(t *testing.T) {
	t.Run("run returns nil", func(t *testing.T) {
		app := New()
		err := app.Run(context.Background(), func(ctx context.Context) error {
			return nil
		})
		assert.NoError(t, err)
	})

	t.Run("run returns error", func(t *testing.T) {
		app := New()
		want := errors.New("run failed")
		err := app.Run(context.Background(), func(ctx context.Context) error {
			return want
		})
		assert.ErrorIs(t, err, want)
	})

	t.Run("shutdown hooks run in LIFO order on context cancel", func(t *testing.T) {
		app := New()
		var mu sync.Mutex
		var order []string

		app.AddShutdownHook(func(ctx context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, "first")
			return nil
		})
		app.AddShutdownHook(func(ctx context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, "second")
			return nil
		})
		app.AddShutdownHook(func(ctx context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, "third")
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		err := app.Run(ctx, func(ctx context.Context) error {
			cancel()
			<-ctx.Done()
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"third", "second", "first"}, order)
	})

	t.Run("hook registered from inside run callback", func(t *testing.T) {
		app := New()
		hookCalled := false

		ctx, cancel := context.WithCancel(context.Background())
		err := app.Run(ctx, func(ctx context.Context) error {
			app.AddShutdownHook(func(ctx context.Context) error {
				hookCalled = true
				return nil
			})
			cancel()
			<-ctx.Done()
			return nil
		})
		require.NoError(t, err)
		assert.True(t, hookCalled)
	})
}
