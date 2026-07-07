package learning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRelearnClearStore_MarkAndRead(t *testing.T) {
	store := NewMemoryRelearnClearStore()
	ctx := context.Background()

	// Empty to start.
	all, err := store.AllClears(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)

	t1 := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store.MarkCleared(ctx, "v\x00nb\x00word", t1))

	all, err = store.AllClears(ctx)
	require.NoError(t, err)
	assert.Equal(t, t1, all["v\x00nb\x00word"])
}

func TestMemoryRelearnClearStore_KeepsLatestClear(t *testing.T) {
	store := NewMemoryRelearnClearStore()
	ctx := context.Background()
	key := "v\x00nb\x00word"

	later := time.Date(2026, 7, 7, 11, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)

	require.NoError(t, store.MarkCleared(ctx, key, later))
	// An older clear must not overwrite a newer one.
	require.NoError(t, store.MarkCleared(ctx, key, earlier))

	all, err := store.AllClears(ctx)
	require.NoError(t, err)
	assert.Equal(t, later, all[key], "the most recent clear time must win")
}

func TestMemoryRelearnClearStore_AllClearsIsACopy(t *testing.T) {
	store := NewMemoryRelearnClearStore()
	ctx := context.Background()
	require.NoError(t, store.MarkCleared(ctx, "k", time.Unix(1, 0)))

	all, err := store.AllClears(ctx)
	require.NoError(t, err)
	all["k"] = time.Unix(999, 0) // mutate the returned map

	again, err := store.AllClears(ctx)
	require.NoError(t, err)
	assert.Equal(t, time.Unix(1, 0).UTC(), again["k"].UTC(), "AllClears must return a defensive copy")
}
