// Package schemas provides embedded SQL migration files.
package schemas

import "embed"

// Migrations contains all SQL migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS
