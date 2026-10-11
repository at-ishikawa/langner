package clicmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/at-ishikawa/langner/internal/auth"
	"github.com/at-ishikawa/langner/internal/config"
	"github.com/at-ishikawa/langner/internal/database"
	"github.com/at-ishikawa/langner/schemas"
)

// PrepareE2EDatabase brings a database to the deterministic e2e baseline for the
// preview seed endpoint, where the deployed server has NO fixture files on disk.
//
// When content != nil (the once-per-run INITIAL seed, which uploads a gzipped
// tar of the fixtures) it does the full setup: apply the schema, import the
// CONTENT from the archive — the only way the serverless backend gets the
// fixtures without a filesystem copy — and provision the allowlist/admin
// accounts. When content == nil (a per-scenario RESET) all three are skipped —
// schema, catalog, and users already exist.
//
// In BOTH cases it resets the learning STATE to the seeded baseline (truncate
// e2eStateTables + reseed the history from the Go fixtures in seed_e2e.go, no
// filesystem). Idempotent: ImportShippedBundle upserts, so re-uploading the same
// content is a no-op beyond the hash check.
func PrepareE2EDatabase(ctx context.Context, cfg *config.Config, db *sqlx.DB, content io.Reader) error {
	// FULL setup only when content is uploaded (the once-per-run initial seed):
	// schema migrate, import content, provision accounts. Per-scenario resets
	// send NO content and skip all three — schema, catalog, and users already
	// exist — so a reset is just a cheap TRUNCATE + reseed. Running migrate (a
	// session-level advisory lock) and provision on every one of ~35 resets
	// churned pooler connections and returned 500 mid-suite.
	if content != nil {
		if err := database.Migrate(db, schemas.Migrations, "migrations"); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
		if _, err := importContentFromTar(ctx, cfg, db, content); err != nil {
			return fmt.Errorf("import notebook content: %w", err)
		}
		if err := provisionAuth(ctx, cfg, db); err != nil {
			return fmt.Errorf("provision auth: %w", err)
		}
	}
	ownerID, err := resolveSeedOwnerID(ctx, cfg, db)
	if err != nil {
		return fmt.Errorf("resolve seed owner: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		"TRUNCATE "+strings.Join(e2eStateTables, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return fmt.Errorf("truncate e2e state: %w", err)
	}
	if err := seedE2EHistory(ctx, db, ownerID); err != nil {
		return fmt.Errorf("seed e2e history: %w", err)
	}
	// On the initial seed, verify content landed: without it there is nothing to
	// quiz, so fail with an actionable message instead of opaque e2e failures.
	if content != nil {
		var files int
		if err := db.GetContext(ctx, &files, "SELECT COUNT(*) FROM notebook_files"); err != nil {
			return fmt.Errorf("check notebook content: %w", err)
		}
		if files == 0 {
			return fmt.Errorf("notebook_files is empty — upload the fixture content archive with the seed request (CI does this), or import it once with `langner-admin migrate seed-e2e`")
		}
	}
	return nil
}

// importContentFromTar extracts a gzipped tar of the notebook fixture tree (the
// CONTENTS of the configured fixtures dir, so each family — stories, flashcards,
// … — is a top-level subdir) into a temp dir, then imports it via the SAME
// ImportShippedContent path the CLI uses, with a cfg copy pointing at that dir.
// Families absent from the archive are simply skipped by ImportShippedContent.
func importContentFromTar(ctx context.Context, cfg *config.Config, db *sqlx.DB, r io.Reader) (int, error) {
	tmp, err := os.MkdirTemp("", "e2e-content-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := extractTarGz(r, tmp); err != nil {
		return 0, fmt.Errorf("extract content archive: %w", err)
	}
	c := *cfg
	c.Notebooks.StoriesDirectories = []string{filepath.Join(tmp, "stories")}
	c.Notebooks.JournalsDirectories = []string{filepath.Join(tmp, "journals")}
	c.Notebooks.FlashcardsDirectories = []string{filepath.Join(tmp, "flashcards")}
	c.Notebooks.DefinitionsDirectories = []string{filepath.Join(tmp, "definitions")}
	c.Notebooks.EtymologyDirectories = []string{filepath.Join(tmp, "etymology")}
	c.Notebooks.GrammarsDirectories = []string{filepath.Join(tmp, "grammars")}
	c.Notebooks.BooksDirectories = nil
	return ImportShippedContent(ctx, &c, db, io.Discard)
}

// extractTarGz safely extracts a gzipped tar stream into dest. It rejects
// absolute paths and any entry that would escape dest (path traversal), skips
// non-regular entries (symlinks etc.), and caps total extracted bytes as a
// modest decompression-bomb guard. Exported-package-internal so it can be unit
// tested without a database.
func extractTarGz(r io.Reader, dest string) error {
	const maxTotalBytes = 256 << 20 // 256 MiB
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		clean := filepath.Clean(hdr.Name)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}
		target := filepath.Join(dest, clean)
		if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			n, cerr := io.Copy(f, tr)
			closeErr := f.Close()
			if cerr != nil {
				return cerr
			}
			if closeErr != nil {
				return closeErr
			}
			total += n
			if total > maxTotalBytes {
				return fmt.Errorf("archive exceeds %d bytes", maxTotalBytes)
			}
		default:
			// skip symlinks / devices / etc. for safety
		}
	}
	return nil
}

// IssueTestToken mints a langner access JWT for email (upserting its user row via
// the same deterministic synthetic sub `auth provision` uses, so the token
// resolves to the account that owns the seeded history), signed with cfg's
// TOKEN_SIGNING_KEY. Shared by `auth issue-test-token` and the seed endpoint.
func IssueTestToken(ctx context.Context, cfg *config.Config, db *sqlx.DB, email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	if !cfg.Auth.Enabled() {
		return "", fmt.Errorf("auth is not enabled (token_signing_key unset)")
	}
	users := auth.NewUserRepository(db)
	userID, err := ensureUser(ctx, users, email)
	if err != nil {
		return "", fmt.Errorf("upsert test user: %w", err)
	}
	tokens, err := auth.NewTokenSigner(auth.DecodeKey(cfg.Auth.TokenSigningKey))
	if err != nil {
		return "", fmt.Errorf("token signing key: %w", err)
	}
	value, err := tokens.Sign(userID, time.Now())
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return value, nil
}
