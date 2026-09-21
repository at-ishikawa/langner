package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// TestCLIRepositories_LivePostgres exercises the device-code and refresh-token
// repositories against a real Postgres: create → approve → consume for a device
// code, and create → find → revoke/family-revoke for refresh tokens. It also
// pins that a plaintext secret is NEVER stored (only its hash) and that the
// approve/consume state guards reject a double-mint.
func TestCLIRepositories_LivePostgres(t *testing.T) {
	dsn := os.Getenv("LANGNER_INTEGRATION_DB_URL")
	if dsn == "" {
		t.Skip("LANGNER_INTEGRATION_DB_URL not set")
	}
	db, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db, schemas.Migrations, "migrations"))

	ctx := context.Background()
	users := NewUserRepository(db)
	user, err := users.Upsert(ctx, "cli-repo-user")
	require.NoError(t, err)

	deviceRepo := NewCLIDeviceCodeRepository(db)
	refreshRepo := NewCLIRefreshTokenRepository(db)

	t.Run("device code create/approve/consume with hashing", func(t *testing.T) {
		deviceCode, err := GenerateDeviceCode()
		require.NoError(t, err)
		userCode, err := GenerateUserCode()
		require.NoError(t, err)
		now := time.Now()
		row, err := deviceRepo.Create(ctx, HashSecret(deviceCode), userCode, now.Add(15*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, DeviceStatusPending, row.Status)

		// Plaintext must never be stored: only the hash is queryable.
		var stored string
		require.NoError(t, db.Get(&stored, `SELECT device_code_hash FROM cli_device_codes WHERE id=$1`, row.ID))
		assert.Equal(t, HashSecret(deviceCode), stored)
		assert.NotEqual(t, deviceCode, stored, "plaintext device code must not be stored")

		// Lookup by hash resolves the same row.
		byHash, err := deviceRepo.FindByDeviceCodeHash(ctx, HashSecret(deviceCode))
		require.NoError(t, err)
		assert.Equal(t, row.ID, byHash.ID)

		// Approve binds the user.
		require.NoError(t, deviceRepo.Approve(ctx, row.ID, user.ID, now))
		approved, err := deviceRepo.FindByUserCode(ctx, userCode)
		require.NoError(t, err)
		assert.Equal(t, DeviceStatusApproved, approved.Status)
		require.True(t, approved.UserID.Valid)
		assert.Equal(t, user.ID, approved.UserID.Int64)

		// Consume once succeeds; a second consume is a no-op error (no re-mint).
		require.NoError(t, deviceRepo.Consume(ctx, row.ID))
		assert.ErrorIs(t, deviceRepo.Consume(ctx, row.ID), ErrDeviceCodeNotFound)
	})

	t.Run("approve rejects an expired code", func(t *testing.T) {
		userCode, _ := GenerateUserCode()
		dc, _ := GenerateDeviceCode()
		now := time.Now()
		row, err := deviceRepo.Create(ctx, HashSecret(dc), userCode, now.Add(-time.Minute)) // already expired
		require.NoError(t, err)
		err = deviceRepo.Approve(ctx, row.ID, user.ID, now)
		assert.ErrorIs(t, err, ErrDeviceCodeNotFound, "an expired code must not be approvable")
	})

	t.Run("refresh token create/find/revoke and family revoke", func(t *testing.T) {
		token, err := GenerateRefreshToken()
		require.NoError(t, err)
		now := time.Now()
		row, err := refreshRepo.Create(ctx, user.ID, HashSecret(token), now.Add(30*24*time.Hour))
		require.NoError(t, err)
		assert.False(t, row.RevokedAt.Valid)

		found, err := refreshRepo.FindByHash(ctx, HashSecret(token))
		require.NoError(t, err)
		assert.Equal(t, row.ID, found.ID)

		require.NoError(t, refreshRepo.Revoke(ctx, row.ID, now))
		reFound, err := refreshRepo.FindByHash(ctx, HashSecret(token))
		require.NoError(t, err)
		assert.True(t, reFound.RevokedAt.Valid, "revoke stamps revoked_at")

		// A second live token, then family revoke kills all live tokens.
		token2, _ := GenerateRefreshToken()
		row2, err := refreshRepo.Create(ctx, user.ID, HashSecret(token2), now.Add(30*24*time.Hour))
		require.NoError(t, err)
		require.NoError(t, refreshRepo.RevokeFamily(ctx, user.ID, now))
		after, err := refreshRepo.FindByHash(ctx, HashSecret(token2))
		require.NoError(t, err)
		assert.True(t, after.RevokedAt.Valid, "family revoke kills row2 (id %d)", row2.ID)
	})

	t.Run("delete expired sweeps only past-expiry rows", func(t *testing.T) {
		dcOld, _ := GenerateDeviceCode()
		ucOld, _ := GenerateUserCode()
		_, err := deviceRepo.Create(ctx, HashSecret(dcOld), ucOld, time.Now().Add(-time.Hour))
		require.NoError(t, err)
		n, err := deviceRepo.DeleteExpired(ctx, time.Now())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, int64(1))
	})
}
