// Package schemas exposes embedded golang-migrate migrations so commands can
// run the schema sync programmatically without depending on the standalone
// migrate CLI being on the user's PATH.
package schemas

import "embed"

// Migrations is the embedded filesystem of *.up.sql / *.down.sql files
// under backend/schemas/migrations.
//
//go:embed migrations/*.sql
var Migrations embed.FS
