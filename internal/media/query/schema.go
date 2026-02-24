package query

import (
	"database/sql"
	"embed"

	"github.com/nao1215/micro/pkg/migration"
)

//go:embed migrations
var migrationsFS embed.FS

// initSchema はマイグレーションを実行してRead Modelのスキーマを適用する。
func initSchema(db *sql.DB) error {
	return migration.Run(db, migrationsFS, "migrations")
}
