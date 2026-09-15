package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// User is an account. No email/name is stored (data minimisation); username is
// an auto-generated, unique, user-facing handle (editable via a later change).
type User struct {
	ID        int64  `db:"id"`
	GoogleSub string `db:"google_sub"`
	Username  string `db:"username"`
}

// UserRepository persists accounts keyed by the opaque Google subject id.
type UserRepository struct {
	db *sqlx.DB
}

// NewUserRepository builds a UserRepository.
func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

// generateUsername returns a random, non-enumerable default handle like
// "user_a1b2c3d4" (5 random bytes → 8 lowercase base32 chars). Uniqueness is
// still enforced by the DB constraint; Upsert regenerates on the rare collision.
func generateUsername() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return "user_" + strings.ToLower(enc.EncodeToString(b)), nil
}

// Upsert find-or-creates the account keyed by google_sub (the stable Google
// subject). Registration is implicit: an unseen google_sub inserts a new row
// with an auto-generated username. A returning google_sub only bumps
// updated_at — the username is PRESERVED (it changes solely through the future
// "update username" path), so the ON CONFLICT branch returns the stored handle.
// The generated username is regenerated on the (astronomically rare) unique
// collision with another account.
func (r *UserRepository) Upsert(ctx context.Context, googleSub string) (*User, error) {
	const maxTries = 5
	for range maxTries {
		username, err := generateUsername()
		if err != nil {
			return nil, err
		}
		var u User
		err = r.db.GetContext(ctx, &u,
			`INSERT INTO users (google_sub, username)
			 VALUES ($1, $2)
			 ON CONFLICT (google_sub) DO UPDATE SET updated_at = CURRENT_TIMESTAMP
			 RETURNING id, google_sub, username`,
			googleSub, username)
		if err == nil {
			return &u, nil
		}
		// A username collision with a DIFFERENT account isn't the google_sub
		// arbiter, so it surfaces as a unique violation — regenerate and retry.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "username") {
			continue
		}
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	return nil, fmt.Errorf("upsert user: could not allocate a unique username after %d attempts", maxTries)
}

// FindByID loads an account by id.
func (r *UserRepository) FindByID(ctx context.Context, id int64) (*User, error) {
	var u User
	if err := r.db.GetContext(ctx, &u,
		`SELECT id, google_sub, username FROM users WHERE id = $1`, id); err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &u, nil
}
