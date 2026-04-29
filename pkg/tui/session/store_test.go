package session_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kubewise/kubewise/pkg/tui/session"
)

func TestStoreSaveAndLoadRecent(t *testing.T) {
	dir := t.TempDir()

	store := &session.Store{Dir: dir}

	sess := session.New()
	sess.Title = "test session"
	sess.Messages = []session.Message{
		{Role: "user", Content: "hello", Timestamp: time.Now()},
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := store.LoadRecent(10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 session, got %d", len(results))
	}
	if results[0].Title != "test session" {
		t.Errorf("title mismatch: got %q", results[0].Title)
	}
}

func TestStoreLoadRecentCapsAtN(t *testing.T) {
	dir := t.TempDir()

	store := &session.Store{Dir: dir}

	for i := range 25 {
		s := session.New()
		s.Title = fmt.Sprintf("session-%02d", i)
		s.ID = fmt.Sprintf("%02d", i)
		if err := store.Save(s); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	results, err := store.LoadRecent(20)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 20 {
		t.Errorf("want exactly 20, got %d", len(results))
	}
}
