package sqlite

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "kubewise-diagnosis-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite3", f.Name()+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE diagnoses (
			id TEXT PRIMARY KEY, cluster_fingerprint TEXT NOT NULL, cluster_display TEXT NOT NULL,
			namespace TEXT NOT NULL, pod TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
			root_cause TEXT, confidence TEXT, evidence TEXT, fix_actions TEXT, impact TEXT,
			duration_ms INTEGER, report_json TEXT, created_at INTEGER NOT NULL)`,
		`CREATE TABLE diagnosis_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT, diagnosis_id TEXT NOT NULL,
			seq_num INTEGER NOT NULL, event_type TEXT NOT NULL, message TEXT DEFAULT '',
			summary TEXT DEFAULT '', detail TEXT DEFAULT '', payload_kind TEXT DEFAULT '',
			payload_json TEXT DEFAULT '', token_in INTEGER DEFAULT 0, token_out INTEGER DEFAULT 0,
			elapsed_ms INTEGER DEFAULT 0, created_at INTEGER NOT NULL,
			UNIQUE(diagnosis_id, seq_num))`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
