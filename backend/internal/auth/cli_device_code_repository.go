package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Device-code status values (mirror the CHECK constraint in migration 029).
const (
	DeviceStatusPending  = "pending"
	DeviceStatusApproved = "approved"
	DeviceStatusDenied   = "denied"
	DeviceStatusConsumed = "consumed"
)

// ErrDeviceCodeNotFound is returned when no device row matches a lookup.
var ErrDeviceCodeNotFound = errors.New("device code not found")

// CLIDeviceCode is one RFC 8628 device-authorization record.
type CLIDeviceCode struct {
	ID             int64         `db:"id"`
	DeviceCodeHash string        `db:"device_code_hash"`
	UserCode       string        `db:"user_code"`
	UserID         sql.NullInt64 `db:"user_id"`
	Status         string        `db:"status"`
	LastPolledAt   sql.NullTime  `db:"last_polled_at"`
	ApprovedAt     sql.NullTime  `db:"approved_at"`
	ExpiresAt      time.Time     `db:"expires_at"`
	CreatedAt      time.Time     `db:"created_at"`
}

// CLIDeviceCodeRepository persists device-authorization records for the CLI
// login flow. It mirrors the UserRepository style (sqlx, explicit columns).
type CLIDeviceCodeRepository struct {
	db *sqlx.DB
}

// NewCLIDeviceCodeRepository builds a CLIDeviceCodeRepository.
func NewCLIDeviceCodeRepository(db *sqlx.DB) *CLIDeviceCodeRepository {
	return &CLIDeviceCodeRepository{db: db}
}

// Create inserts a pending device record (user_id NULL until approval).
func (r *CLIDeviceCodeRepository) Create(ctx context.Context, deviceCodeHash, userCode string, expiresAt time.Time) (*CLIDeviceCode, error) {
	var row CLIDeviceCode
	if err := r.db.GetContext(ctx, &row,
		`INSERT INTO cli_device_codes (device_code_hash, user_code, status, expires_at)
		 VALUES ($1, $2, 'pending', $3)
		 RETURNING id, device_code_hash, user_code, user_id, status, last_polled_at, approved_at, expires_at, created_at`,
		deviceCodeHash, userCode, expiresAt); err != nil {
		return nil, fmt.Errorf("create device code: %w", err)
	}
	return &row, nil
}

// FindByUserCode loads a device record by its human user_code (approval page).
func (r *CLIDeviceCodeRepository) FindByUserCode(ctx context.Context, userCode string) (*CLIDeviceCode, error) {
	return r.findBy(ctx, `user_code = $1`, userCode)
}

// FindByDeviceCodeHash loads a device record by the SHA-256 of the presented
// device_code (token poll).
func (r *CLIDeviceCodeRepository) FindByDeviceCodeHash(ctx context.Context, deviceCodeHash string) (*CLIDeviceCode, error) {
	return r.findBy(ctx, `device_code_hash = $1`, deviceCodeHash)
}

func (r *CLIDeviceCodeRepository) findBy(ctx context.Context, whereClause string, arg any) (*CLIDeviceCode, error) {
	var row CLIDeviceCode
	err := r.db.GetContext(ctx, &row,
		`SELECT id, device_code_hash, user_code, user_id, status, last_polled_at, approved_at, expires_at, created_at
		 FROM cli_device_codes WHERE `+whereClause, arg)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceCodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find device code: %w", err)
	}
	return &row, nil
}

// Approve binds a user to a still-pending, unexpired device row and marks it
// approved. It is a no-op-safe conditional update: it only touches a row that
// is still pending and unexpired, returning ErrDeviceCodeNotFound otherwise so
// a consumed/expired/denied code cannot be re-approved.
func (r *CLIDeviceCodeRepository) Approve(ctx context.Context, id, userID int64, now time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cli_device_codes
		 SET user_id = $1, status = 'approved', approved_at = $2
		 WHERE id = $3 AND status = 'pending' AND expires_at > $2`,
		userID, now, id)
	if err != nil {
		return fmt.Errorf("approve device code: %w", err)
	}
	return oneRow(res)
}

// Deny marks a pending device row denied (no identity needed to deny).
func (r *CLIDeviceCodeRepository) Deny(ctx context.Context, userCode string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cli_device_codes SET status = 'denied'
		 WHERE user_code = $1 AND status = 'pending'`, userCode)
	if err != nil {
		return fmt.Errorf("deny device code: %w", err)
	}
	return oneRow(res)
}

// MarkPolled records the time of a poll for slow_down enforcement.
func (r *CLIDeviceCodeRepository) MarkPolled(ctx context.Context, id int64, at time.Time) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE cli_device_codes SET last_polled_at = $1 WHERE id = $2`, at, id); err != nil {
		return fmt.Errorf("mark device code polled: %w", err)
	}
	return nil
}

// Consume marks an approved device row consumed so the same device_code cannot
// mint a second token. Only an approved row transitions, guarding double-spend.
func (r *CLIDeviceCodeRepository) Consume(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE cli_device_codes SET status = 'consumed'
		 WHERE id = $1 AND status = 'approved'`, id)
	if err != nil {
		return fmt.Errorf("consume device code: %w", err)
	}
	return oneRow(res)
}

// DeleteExpired removes device rows whose expiry has passed (sweeper).
func (r *CLIDeviceCodeRepository) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cli_device_codes WHERE expires_at < $1`, now)
	if err != nil {
		return 0, fmt.Errorf("delete expired device codes: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func oneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrDeviceCodeNotFound
	}
	return nil
}
