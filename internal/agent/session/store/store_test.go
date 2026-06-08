package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kubewise/kubewise/internal/agent/session"
	"github.com/kubewise/kubewise/internal/agent/session/store"
)

func TestStoreSaveAndLoadRecent(t *testing.T) {
	dir := t.TempDir()

	s := &store.FileStore{Dir: dir}

	conv := session.NewConversation()
	conv.Title = "test conversation"
	conv.Messages = []session.Message{
		{Role: "user", Content: "hello", Timestamp: time.Now()},
	}

	if err := s.Save(conv); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := s.LoadRecent(10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(results))
	}
	if results[0].Title != "test conversation" {
		t.Errorf("title mismatch: got %q", results[0].Title)
	}
}

func TestStoreLoadRecentCapsAtN(t *testing.T) {
	dir := t.TempDir()

	s := &store.FileStore{Dir: dir}

	for i := range 25 {
		conv := session.NewConversation()
		conv.Title = fmt.Sprintf("conversation-%02d", i)
		conv.ID = fmt.Sprintf("%02d", i)
		if err := s.Save(conv); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	results, err := s.LoadRecent(20)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(results) != 20 {
		t.Errorf("want exactly 20, got %d", len(results))
	}
}
