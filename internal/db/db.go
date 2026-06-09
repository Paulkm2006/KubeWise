package db

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at the given dir.
func Open(dir string) (*DB, error) {
	path := filepath.Join(dir, "kubewise.db")
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	db := &DB{conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) migrate() error {
	data, err := migrations.ReadFile("migrations/001_initial.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	_, err = db.Exec(string(data))
	return err
}
