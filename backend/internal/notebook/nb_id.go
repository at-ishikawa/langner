package notebook

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// NotebookIDPrefix is the prefix every server-minted user-notebook id carries.
// It makes the nb_ namespace disjoint from every existing human-authored id
// (the `id:` in a shipped index.yml), so an nb_ id can never collide with a
// shipped one — the two coexist in the single flat notebook-id namespace.
const NotebookIDPrefix = "nb_"

// base62 alphabet for the random suffix (URL-safe, opaque, case-sensitive).
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// nbIDRandomLen is the number of base62 chars after the prefix. 22 base62
// chars carry ~130 bits of entropy, so collisions are astronomically unlikely;
// the notebooks PK still makes minting collision-safe by construction.
const nbIDRandomLen = 22

// MintNotebookID returns a fresh opaque `nb_` id: the prefix plus 22 base62
// characters drawn from a CSPRNG. It is minted server-side in the push handler
// (never client-supplied — the app owns the id namespace). Uniqueness is
// guaranteed by the notebooks primary key; the caller retries on the
// (astronomically-unlikely) 23505 conflict.
func MintNotebookID() (string, error) {
	var b strings.Builder
	b.WriteString(NotebookIDPrefix)
	max := big.NewInt(int64(len(base62Alphabet)))
	for i := 0; i < nbIDRandomLen; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("read random for notebook id: %w", err)
		}
		b.WriteByte(base62Alphabet[n.Int64()])
	}
	return b.String(), nil
}

// IsMintedNotebookID reports whether id was server-minted (carries the nb_
// prefix). Used to distinguish user notebooks from shipped ones where a code
// path needs to branch on provenance without a DB round-trip.
func IsMintedNotebookID(id string) bool {
	return strings.HasPrefix(id, NotebookIDPrefix)
}
