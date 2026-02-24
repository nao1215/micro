package notification

import (
	"database/sql"
	"embed"

	"github.com/nao1215/micro/pkg/migration"
)

//go:embed migrations
var migrationsFS embed.FS

// initSchema はマイグレーションを実行してSQLiteデータベースにスキーマを適用する。
func initSchema(db *sql.DB) error {
	return migration.Run(db, migrationsFS, "migrations")
}
