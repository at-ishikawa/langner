package auth

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// User is a decrypted account. Email and Name are plaintext for in-process use
// only (display, allowlist echo); at rest they are AES-256-GCM ciphertext.
type User struct {
	ID        int64
	GoogleSub string
	Email     string
	Name      string
}

// userRow is the on-disk shape: PII columns hold ciphertext.
type userRow struct {
	ID             int64  `db:"id"`
	GoogleSub      string `db:"google_sub"`
	EmailEncrypted []byte `db:"email_encrypted"`
	NameEncrypted  []byte `db:"name_encrypted"`
}

// UserRepository persists accounts with PII encrypted at rest.
type UserRepository struct {
	db  *sqlx.DB
	enc *Encryptor
}

// NewUserRepository builds a UserRepository.
func NewUserRepository(db *sqlx.DB, enc *Encryptor) *UserRepository {
	return &UserRepository{db: db, enc: enc}
}

// Upsert find-or-updates the account keyed by google_sub (the stable Google
// subject). It re-encrypts email/name and recomputes the blind index on every
// sign-in so a changed display name is reflected. Registration is implicit: an
// unseen google_sub inserts a new row. The race-safe INSERT ... ON CONFLICT ...
// DO UPDATE ... RETURNING idiom mirrors the note upsert in internal/notebook.
func (r *UserRepository) Upsert(ctx context.Context, googleSub, email, name string) (*User, error) {
	emailEnc, err := r.enc.Encrypt(email)
	if err != nil {
		return nil, fmt.Errorf("encrypt email: %w", err)
	}
	nameEnc, err := r.enc.Encrypt(name)
	if err != nil {
		return nil, fmt.Errorf("encrypt name: %w", err)
	}
	emailHash := r.enc.BlindIndex(email)

	var id int64
	err = r.db.GetContext(ctx, &id,
		`INSERT INTO users (google_sub, email_encrypted, email_hash, name_encrypted)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (google_sub) DO UPDATE SET
		     email_encrypted = EXCLUDED.email_encrypted,
		     email_hash = EXCLUDED.email_hash,
		     name_encrypted = EXCLUDED.name_encrypted
		 RETURNING id`,
		googleSub, emailEnc, emailHash, nameEnc)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	return &User{ID: id, GoogleSub: googleSub, Email: email, Name: name}, nil
}

// FindByID loads an account by id, decrypting email and name for display.
func (r *UserRepository) FindByID(ctx context.Context, id int64) (*User, error) {
	var row userRow
	if err := r.db.GetContext(ctx, &row,
		`SELECT id, google_sub, email_encrypted, name_encrypted FROM users WHERE id = $1`, id); err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return r.decrypt(row)
}

// FindByEmail loads an account by its blind-index email hash.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var row userRow
	if err := r.db.GetContext(ctx, &row,
		`SELECT id, google_sub, email_encrypted, name_encrypted FROM users WHERE email_hash = $1`,
		r.enc.BlindIndex(email)); err != nil {
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return r.decrypt(row)
}

func (r *UserRepository) decrypt(row userRow) (*User, error) {
	email, err := r.enc.Decrypt(row.EmailEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt email: %w", err)
	}
	name, err := r.enc.Decrypt(row.NameEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt name: %w", err)
	}
	return &User{ID: row.ID, GoogleSub: row.GoogleSub, Email: email, Name: name}, nil
}
