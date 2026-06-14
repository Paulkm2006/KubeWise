package sqlite

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "kubewise-activityfeed-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite", f.Name()+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE activities (
		id TEXT PRIMARY KEY, type TEXT NOT NULL, text TEXT NOT NULL,
		cluster_display TEXT, diagnosis_id TEXT, created_at INTEGER NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
