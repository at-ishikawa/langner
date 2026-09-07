package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/inference"
)

// LLMCredential is a user's decrypted LLM provider credential. APIKey is
// plaintext for in-process use only (the outbound LLM call); at rest it is
// AES-256-GCM ciphertext. Model may be empty (the factory applies a default).
type LLMCredential struct {
	Provider string
	APIKey   string
	Model    string
}

// llmCredentialRow is the on-disk shape: the API key column holds ciphertext.
type llmCredentialRow struct {
	Provider        string `db:"provider"`
	APIKeyEncrypted []byte `db:"api_key_encrypted"`
	Model           string `db:"model"`
}

// CredentialsRepository persists per-user LLM credentials with the API key
// encrypted at rest, mirroring UserRepository (same {db, enc} pair and the same
// Encryptor / CREDENTIAL_ENCRYPTION_KEY).
type CredentialsRepository struct {
	db  *sqlx.DB
	enc *Encryptor
}

// NewCredentialsRepository builds a CredentialsRepository.
func NewCredentialsRepository(db *sqlx.DB, enc *Encryptor) *CredentialsRepository {
	return &CredentialsRepository{db: db, enc: enc}
}

// SetCredential stores (or replaces) the user's LLM credential, encrypting the
// API key. There is one credential per user (user_id PRIMARY KEY): registering
// a new provider or rotating the key overwrites the existing row. The provider
// must be one inference.AvailableProviders() advertises.
func (r *CredentialsRepository) SetCredential(ctx context.Context, userID int64, provider, apiKey, model string) error {
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
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO user_llm_credentials (user_id, provider, api_key_encrypted, model)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE SET
		     provider = EXCLUDED.provider,
		     api_key_encrypted = EXCLUDED.api_key_encrypted,
		     model = EXCLUDED.model`,
		userID, provider, keyEnc, model); err != nil {
		return fmt.Errorf("upsert llm credential: %w", err)
	}
	return nil
}

// GetCredential loads and decrypts the user's LLM credential. The bool reports
// whether one is configured (false with a nil error when the user has none).
func (r *CredentialsRepository) GetCredential(ctx context.Context, userID int64) (*LLMCredential, bool, error) {
	var row llmCredentialRow
	err := r.db.GetContext(ctx, &row,
		`SELECT provider, api_key_encrypted, model FROM user_llm_credentials WHERE user_id = $1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get llm credential: %w", err)
	}
	apiKey, err := r.enc.Decrypt(row.APIKeyEncrypted)
	if err != nil {
		return nil, false, fmt.Errorf("decrypt api key: %w", err)
	}
	return &LLMCredential{Provider: row.Provider, APIKey: apiKey, Model: row.Model}, true, nil
}

// DeleteCredential clears the user's LLM credential. Deleting a non-existent
// credential is a no-op (no error), so the endpoint is idempotent.
func (r *CredentialsRepository) DeleteCredential(ctx context.Context, userID int64) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_llm_credentials WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete llm credential: %w", err)
	}
	return nil
}
