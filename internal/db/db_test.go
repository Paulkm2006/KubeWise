package db

import (
	"os"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dir, _ := os.MkdirTemp("", "kubewise-db-test")
	defer os.RemoveAll(dir)

	d, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() err = %v", err)
	}
	defer d.Close()

	// Verify diagnoses table exists
	var name string
	err = d.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='diagnoses'").Scan(&name)
	if err != nil {
		t.Fatalf("diagnoses table not found: %v", err)
	}
}
