package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/inference"
)

// LLMCredential is a user's decrypted LLM provider credential. APIKey is
// plaintext for in-process use only (the outbound LLM call); at rest it is
// AES-256-GCM ciphertext. Model may be empty (the factory applies a default).
// IsPrimary marks the provider the grader uses first; the other configured
// provider is the automatic rate-limit/quota fallback.
type LLMCredential struct {
	Provider  string
	APIKey    string
	Model     string
	IsPrimary bool
}

// CredentialMeta is the non-secret view of a stored credential (no API key),
// for /auth/me and the Settings UI.
type CredentialMeta struct {
	Provider  string `db:"provider"`
	Model     string `db:"model"`
	IsPrimary bool   `db:"is_primary"`
}

// llmCredentialRow is the on-disk shape: the API key column holds ciphertext.
type llmCredentialRow struct {
	Provider        string `db:"provider"`
	APIKeyEncrypted []byte `db:"api_key_encrypted"`
	Model           string `db:"model"`
	IsPrimary       bool   `db:"is_primary"`
}

// CredentialsRepository persists per-user LLM credentials with the API key
// encrypted at rest. A user may register one credential PER provider (keyed by
// (user_id, provider)); exactly one is the primary. Mirrors UserRepository
// (same {db, enc} pair, same Encryptor / CREDENTIAL_ENCRYPTION_KEY).
type CredentialsRepository struct {
	db  *sqlx.DB
	enc *Encryptor
}

// NewCredentialsRepository builds a CredentialsRepository.
func NewCredentialsRepository(db *sqlx.DB, enc *Encryptor) *CredentialsRepository {
	return &CredentialsRepository{db: db, enc: enc}
}

// SetCredential stores (or rotates) the user's key for one provider, encrypting
// it. makePrimary — or registering the user's FIRST credential — makes this the
// primary provider (clearing any previous primary). The provider must be one
// inference.AvailableProviders() advertises.
func (r *CredentialsRepository) SetCredential(ctx context.Context, userID int64, provider, apiKey, model string, makePrimary bool) error {
	if !inference.IsAvailableProvider(provider) {
		return fmt.Errorf("unsupported provider %q", provider)
	}
	if apiKey == "" {
		return errors.New("api key is required")
	}
	keyEnc, err := r.enc.Encrypt(apiKey)
	if err != nil {
		return fmt.Errorf("encrypt api key: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_llm_credentials (user_id, provider, api_key_encrypted, model)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, provider) DO UPDATE SET
		     api_key_encrypted = EXCLUDED.api_key_encrypted,
		     model = EXCLUDED.model`,
		userID, provider, keyEnc, model); err != nil {
		return fmt.Errorf("upsert llm credential: %w", err)
	}

	var hasPrimary bool
	if err := tx.GetContext(ctx, &hasPrimary,
		`SELECT EXISTS(SELECT 1 FROM user_llm_credentials WHERE user_id = $1 AND is_primary)`, userID); err != nil {
		return fmt.Errorf("check primary: %w", err)
	}
	if makePrimary || !hasPrimary {
		if err := r.setPrimaryTx(ctx, tx, userID, provider); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set credential: %w", err)
	}
	return nil
}

// SetPrimaryProvider makes an ALREADY-registered provider the user's primary
// (no key needed). Errors if the user has no credential for that provider.
func (r *CredentialsRepository) SetPrimaryProvider(ctx context.Context, userID int64, provider string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM user_llm_credentials WHERE user_id = $1 AND provider = $2)`, userID, provider); err != nil {
		return fmt.Errorf("check credential: %w", err)
	}
	if !exists {
		return fmt.Errorf("no %s credential to make primary", provider)
	}
	if err := r.setPrimaryTx(ctx, tx, userID, provider); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set primary: %w", err)
	}
	return nil
}

// setPrimaryTx flips is_primary so exactly `provider` is primary for the user.
// It clears every primary first, then sets the one — two statements in the tx,
// not `SET is_primary = (provider = $2)` in one. The one-primary index
// (idx_user_llm_credentials_primary) is a non-deferrable partial unique index,
// so a single multi-row UPDATE can transiently hold two primary rows (the new
// one set true before the old is cleared) and the index rejects it mid-statement
// (SQLSTATE 23505). Clearing all to false first means the set-true step never
// collides.
func (r *CredentialsRepository) setPrimaryTx(ctx context.Context, tx *sqlx.Tx, userID int64, provider string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_llm_credentials SET is_primary = FALSE WHERE user_id = $1 AND is_primary`,
		userID); err != nil {
		return fmt.Errorf("clear primary provider: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_llm_credentials SET is_primary = TRUE WHERE user_id = $1 AND provider = $2`,
		userID, provider); err != nil {
		return fmt.Errorf("set primary provider: %w", err)
	}
	return nil
}

// ListCredentials returns the user's credentials without keys (primary first),
// for /auth/me and the Settings UI.
func (r *CredentialsRepository) ListCredentials(ctx context.Context, userID int64) ([]CredentialMeta, error) {
	var metas []CredentialMeta
	if err := r.db.SelectContext(ctx, &metas,
		`SELECT provider, model, is_primary FROM user_llm_credentials
		 WHERE user_id = $1 ORDER BY is_primary DESC, provider`, userID); err != nil {
		return nil, fmt.Errorf("list llm credentials: %w", err)
	}
	return metas, nil
}

// DecryptedCredentials loads and decrypts all of the user's credentials
// (primary first), for the resolver to build the primary client + fallback.
func (r *CredentialsRepository) DecryptedCredentials(ctx context.Context, userID int64) ([]LLMCredential, error) {
	var rows []llmCredentialRow
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT provider, api_key_encrypted, model, is_primary FROM user_llm_credentials
		 WHERE user_id = $1 ORDER BY is_primary DESC, provider`, userID); err != nil {
		return nil, fmt.Errorf("get llm credentials: %w", err)
	}
	creds := make([]LLMCredential, 0, len(rows))
	for _, row := range rows {
		apiKey, err := r.enc.Decrypt(row.APIKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s api key: %w", row.Provider, err)
		}
		creds = append(creds, LLMCredential{Provider: row.Provider, APIKey: apiKey, Model: row.Model, IsPrimary: row.IsPrimary})
	}
	return creds, nil
}

// DeleteCredential removes the user's credential for one provider. If it was the
// primary and another provider remains, the lexically-first remaining provider
// is promoted to primary so a user with any key always has a primary. Deleting a
// non-existent credential is a no-op (idempotent).
func (r *CredentialsRepository) DeleteCredential(ctx context.Context, userID int64, provider string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_llm_credentials WHERE user_id = $1 AND provider = $2`, userID, provider); err != nil {
		return fmt.Errorf("delete llm credential: %w", err)
	}
	var hasPrimary bool
	if err := tx.GetContext(ctx, &hasPrimary,
		`SELECT EXISTS(SELECT 1 FROM user_llm_credentials WHERE user_id = $1 AND is_primary)`, userID); err != nil {
		return fmt.Errorf("check primary: %w", err)
	}
	if !hasPrimary {
		if _, err := tx.ExecContext(ctx,
			`UPDATE user_llm_credentials SET is_primary = TRUE
			 WHERE user_id = $1
			   AND provider = (SELECT MIN(provider) FROM user_llm_credentials WHERE user_id = $1)`, userID); err != nil {
			return fmt.Errorf("promote primary: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete credential: %w", err)
	}
	return nil
}
