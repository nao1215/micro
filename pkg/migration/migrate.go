// Package migration はSQLiteデータベースのマイグレーションを管理する。
// embed.FSからSQLファイルを読み込み、バージョン管理テーブルで適用状態を追跡する。
package migration

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"strings"
)

// Run はembedされたマイグレーションファイルを順序通りに適用する。
// 未適用のマイグレーションのみ実行し、適用済みのものはスキップする。
// ファイル名形式: 000001_description.up.sql
func Run(db *sql.DB, fsys fs.FS, dir string) error {
	if err := ensureMigrationsTable(db); err != nil {
		return fmt.Errorf("マイグレーション管理テーブルの作成に失敗: %w", err)
	}

	applied, err := getAppliedVersions(db)
	if err != nil {
		return fmt.Errorf("適用済みバージョンの取得に失敗: %w", err)
	}

	migrations, err := collectMigrations(fsys, dir)
	if err != nil {
		return fmt.Errorf("マイグレーションファイルの収集に失敗: %w", err)
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		if err := applyMigration(db, fsys, m); err != nil {
			return fmt.Errorf("マイグレーション %06d の適用に失敗: %w", m.version, err)
		}
		log.Printf("[Migration] マイグレーション %06d_%s を適用しました", m.version, m.name)
	}

	return nil
}

type migrationFile struct {
	version int
	name    string
	path    string
}

// ensureMigrationsTable はバージョン管理テーブルを作成する。
func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	return err
}

// getAppliedVersions は適用済みのマイグレーションバージョンを取得する。
func getAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// collectMigrations はディレクトリからup.sqlファイルを収集してバージョン順にソートする。
func collectMigrations(fsys fs.FS, dir string) ([]migrationFile, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}

	var migrations []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(parts[1], ".up.sql")
		migrations = append(migrations, migrationFile{
			version: version,
			name:    name,
			path:    dir + "/" + entry.Name(),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

// applyMigration は1つのマイグレーションをトランザクション内で適用する。
func applyMigration(db *sql.DB, fsys fs.FS, m migrationFile) error {
	content, err := fs.ReadFile(fsys, m.path)
	if err != nil {
		return fmt.Errorf("ファイル読み込みに失敗: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("トランザクション開始に失敗: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(string(content)); err != nil {
		return fmt.Errorf("SQL実行に失敗: %w", err)
	}

	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
		return fmt.Errorf("バージョン記録に失敗: %w", err)
	}

	return tx.Commit()
}
