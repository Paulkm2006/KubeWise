package diagnosis

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/db"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	conn, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	conn.SetMaxOpenConns(1) // single connection so :memory: databases don't split
	db.DB = conn

	// Run migrations
	data001, err := os.ReadFile("../db/migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read migration 001: %v", err)
	}
	data002, err := os.ReadFile("../db/migrations/002_diagnosis_events.sql")
	if err != nil {
		t.Fatalf("read migration 002: %v", err)
	}
	if _, err := conn.Exec(string(data001)); err != nil {
		t.Fatalf("exec migration 001: %v", err)
	}
	if _, err := conn.Exec(string(data002)); err != nil {
		t.Fatalf("exec migration 002: %v", err)
	}

	// Add the status column if it doesn't exist (normally done by db.Open via
	// ensureDiagnosisStatusColumn, but we skip that here).
	var count int
	conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('diagnoses') WHERE name='status'").Scan(&count)
	if count == 0 {
		if _, err := conn.Exec("ALTER TABLE diagnoses ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'"); err != nil {
			t.Fatalf("add status column: %v", err)
		}
	}
}

func TestRunnerStartAndStatus(t *testing.T) {
	setupTestDB(t)
	r := NewRunner()
	ctx := context.Background()

	id := "test-1"
	err := r.Start(ctx, id, DiagnosisTarget{
		Cluster: "c1", ClusterDisplay: "Cluster 1", Namespace: "ns1", Pod: "pod1",
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	d, err := r.GetStatus(ctx, id)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if d.Status != StatusRunning {
		t.Fatalf("expected running, got %s", d.Status)
	}
}

func TestRunnerPushEventAndGetEventsSince(t *testing.T) {
	setupTestDB(t)
	r := NewRunner()
	ctx := context.Background()

	id := "test-2"
	r.Start(ctx, id, DiagnosisTarget{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "p"})

	r.PushEvent(ctx, id, event.Phase{QueryID: "q1", Phase: "collecting context"})
	r.PushEvent(ctx, id, event.ToolCall{QueryID: "q1", ToolName: "get_pod_logs", Step: 1})

	events, err := r.GetEventsSince(ctx, id, 0)
	if err != nil {
		t.Fatalf("GetEventsSince failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "phase" || events[0].Message != "collecting context" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].EventType != "tool_call" || events[1].Message != "get_pod_logs" {
		t.Fatalf("unexpected second event: %+v", events[1])
	}

	// Test GetEventsSince with a seq_num filter
	later, _ := r.GetEventsSince(ctx, id, 1)
	if len(later) != 1 {
		t.Fatalf("expected 1 event since seq 1, got %d", len(later))
	}
}

func TestRunnerSetCompletedAndFailed(t *testing.T) {
	setupTestDB(t)
	r := NewRunner()
	ctx := context.Background()

	id := "test-3"
	r.Start(ctx, id, DiagnosisTarget{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "p"})

	result := &DiagnosisResult{
		RootCause:  "OOMKilled",
		Confidence: "high",
		Evidence:   []Evidence{{Num: 1, Text: "Container exited with code 137"}},
		FixActions: []FixAction{{
			Type: "command", Description: "Increase memory limit",
			Command: "kubectl set resources pod/p --limits=memory=512Mi",
		}},
		Impact:     "Pod restarting",
		DurationMs: 12300,
	}
	r.SetCompleted(ctx, id, result)

	d, _ := r.GetStatus(ctx, id)
	if d.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", d.Status)
	}
	if d.RootCause != "OOMKilled" {
		t.Fatalf("expected OOMKilled, got %s", d.RootCause)
	}
	if len(d.Evidence) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(d.Evidence))
	}

	// Test failed
	id2 := "test-4"
	r.Start(ctx, id2, DiagnosisTarget{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "p"})
	r.SetFailed(ctx, id2, "agent timeout")

	d2, _ := r.GetStatus(ctx, id2)
	if d2.Status != StatusFailed {
		t.Fatalf("expected failed, got %s", d2.Status)
	}
}

func TestRunnerList(t *testing.T) {
	setupTestDB(t)
	r := NewRunner()
	ctx := context.Background()

	r.Start(ctx, "l1", DiagnosisTarget{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "p1"})
	r.Start(ctx, "l2", DiagnosisTarget{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "p2"})
	r.SetCompleted(ctx, "l2", &DiagnosisResult{RootCause: "ok"})

	list, total, err := r.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 diagnoses, got %d", len(list))
	}
}