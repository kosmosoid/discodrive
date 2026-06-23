// Package discodrive embeds repository-level assets (DB migrations)
// so the single binary carries them along.
package discodrive

import "embed"

// Migrations holds the schema SQL migrations (golang-migrate, format NNNNNN_name.{up,down}.sql).
//
//go:embed migrations/*.sql
var Migrations embed.FS
