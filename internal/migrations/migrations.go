// Package migrations embeds the .sql files in this directory so the
// sim binary can self-bootstrap a fresh database without requiring
// the user to invoke the goose CLI separately.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
