// Package migrations содержит SQL-миграции golang-migrate, встроенные в
// бинарник (NFR-5: миграции применяются при старте API и командой make migrate).
package migrations

import "embed"

// FS — встроенная файловая система с *.up.sql / *.down.sql.
//
//go:embed *.sql
var FS embed.FS
