package activity

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, _ := os.CreateTemp("", "kubewise-activity-test-*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite3", f.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS activities (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		text TEXT NOT NULL,
		cluster_display TEXT,
		diagnosis_id TEXT,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}

	return db
}

func TestAddAndList(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db)

	a, err := svc.Add(context.Background(), TypeDiagnosis, "nginx OOMKilled", "prod-us", "diag-1")
	if err != nil {
		t.Fatalf("Add() err = %v", err)
	}
	if a.Type != TypeDiagnosis {
		t.Fatalf("expected TypeDiagnosis, got %v", a.Type)
	}

	activities, err := svc.List(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("List() err = %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}
	if activities[0].ClusterDisplay != "prod-us" {
		t.Fatalf("expected cluster prod-us, got %s", activities[0].ClusterDisplay)
	}
}
