package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/kubewise/kubewise/internal/diagnosis/domain"
	diagsqlite "github.com/kubewise/kubewise/internal/diagnosis/infrastructure/sqlite"
	"github.com/kubewise/kubewise/internal/platform/agentruntime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose/report"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
)

type fakeDiagnosisRunner struct{}

func (f *fakeDiagnosisRunner) DiagnosePod(_ context.Context, _ agentruntime.DiagnoseParams, _ string, ch chan<- agentruntime.ProgressEvent) error {
	diagReport := report.DiagnosisReport{
		Verdict: report.VerdictConfirmed,
		RootCause: report.RootCause{
			Category: "oom_killed", Title: "OOM", Summary: "memory limit exceeded",
			ConfidenceScore: 0.91, ConfidenceLabel: "high",
		},
		Evidence: []report.Evidence{
			{ID: "e1", Source: "container_status", Strength: "strong", Summary: "OOMKilled"},
		},
		Limitations: []string{"metrics not verified"},
	}
	payload, _ := json.Marshal(diagReport)
	ch <- agentruntime.ProgressEvent{Type: "phase", Message: "diagnosis.collect", Summary: "collecting"}
	ch <- agentruntime.ProgressEvent{
		Type: "agent_done", Result: "## Diagnosis\nOOM",
		Summary: "done", PayloadKind: event.PayloadKindDiagnosisReport, PayloadJSON: string(payload),
	}
	return nil
}

type blockingDiagnosisRunner struct {
	started chan struct{}
}

func (f *blockingDiagnosisRunner) DiagnosePod(ctx context.Context, _ agentruntime.DiagnoseParams, _ string, ch chan<- agentruntime.ProgressEvent) error {
	close(f.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestServiceStartPersistsEventsDuringRun(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(diagsqlite.NewRepository(db), &fakeDiagnosisRunner{}, nil)
	ctx := context.Background()

	id, err := svc.Start(ctx, domain.Target{Cluster: "c1", ClusterDisplay: "c1", Namespace: "default", Pod: "nginx"})
	if err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		events, _ := svc.EventsSince(ctx, id, 0)
		if len(events) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for events")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServiceLatestReturnsNewestDiagnosis(t *testing.T) {
	db := openTestDB(t)
	repo := diagsqlite.NewRepository(db)
	svc := NewService(repo, &fakeDiagnosisRunner{}, nil)
	ctx := context.Background()
	target := domain.Target{Cluster: "c1", ClusterDisplay: "c1", Namespace: "default", Pod: "nginx"}

	if _, _, err := svc.Latest(ctx, target); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	id1, err := svc.Start(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, svc, id1, domain.StatusCompleted)

	time.Sleep(1100 * time.Millisecond)

	id2, err := svc.Start(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, svc, id2, domain.StatusCompleted)

	latest, events, err := svc.Latest(ctx, target)
	if err != nil {
		t.Fatalf("Latest() err = %v", err)
	}
	if latest.ID != id2 {
		t.Fatalf("expected latest %s, got %s", id2, latest.ID)
	}
	if len(events) == 0 {
		t.Fatal("expected events for latest diagnosis")
	}
	if latest.Report == nil || latest.Report.Verdict != domain.VerdictConfirmed {
		t.Fatalf("expected structured report on latest diagnosis")
	}
}

func TestServiceCancelStopsRunningDiagnosis(t *testing.T) {
	db := openTestDB(t)
	repo := diagsqlite.NewRepository(db)
	runner := &blockingDiagnosisRunner{started: make(chan struct{})}
	svc := NewService(repo, runner, nil)
	ctx := context.Background()
	target := domain.Target{Cluster: "c1", ClusterDisplay: "c1", Namespace: "default", Pod: "nginx"}

	id, err := svc.Start(ctx, target)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for diagnosis to start")
	}

	if err := svc.Cancel(ctx, id); err != nil {
		t.Fatalf("Cancel() err = %v", err)
	}

	waitForStatus(t, svc, id, domain.StatusCancelled)
}

func TestParseResultFromProgress(t *testing.T) {
	diagReport := report.DiagnosisReport{
		Verdict:   report.VerdictLikely,
		RootCause: report.RootCause{Summary: "likely root cause", ConfidenceLabel: "medium"},
	}
	raw, _ := json.Marshal(diagReport)
	got := parseResultFromProgress(agentruntime.ProgressEvent{
		Type: "agent_done", Result: "markdown",
		PayloadKind: event.PayloadKindDiagnosisReport, PayloadJSON: string(raw),
	}, 1200)
	if got == nil || got.Verdict != domain.VerdictLikely {
		t.Fatalf("unexpected parse result: %+v", got)
	}
}

func waitForStatus(t *testing.T, svc *Service, id string, want domain.Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		d, err := svc.Status(context.Background(), id)
		if err != nil {
			t.Fatalf("Status() err = %v", err)
		}
		if d.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for status %s, got %s", want, d.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	f, _ := os.CreateTemp("", "diag-svc-*.db")
	t.Cleanup(func() { os.Remove(f.Name()) })
	db, _ := sql.Open("sqlite3", f.Name())
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
