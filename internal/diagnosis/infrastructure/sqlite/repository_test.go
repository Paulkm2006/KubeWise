package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kubewise/kubewise/internal/diagnosis/domain"
)

func TestRepositoryLifecycle(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	id := "test-1"
	if err := repo.Create(ctx, id, domain.Target{
		Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "pod1",
	}); err != nil {
		t.Fatalf("Create() err = %v", err)
	}

	d, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID() err = %v", err)
	}
	if d.Status != domain.StatusRunning {
		t.Fatalf("expected running, got %s", d.Status)
	}

	if err := repo.AppendEvent(ctx, id, domain.EventAppend{EventType: "phase", Message: "collecting"}); err != nil {
		t.Fatal(err)
	}
	events, err := repo.ListEventsSince(ctx, id, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected 1 event, got %d err=%v", len(events), err)
	}
}

func TestRepositoryGetLatestByTarget(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	target := domain.Target{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "pod1"}

	if _, err := repo.GetLatestByTarget(ctx, target); err != sql.ErrNoRows {
		t.Fatalf("expected no rows, got %v", err)
	}

	if err := repo.Create(ctx, "diag-a", target); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := repo.Create(ctx, "diag-z", target); err != nil {
		t.Fatal(err)
	}

	latest, err := repo.GetLatestByTarget(ctx, target)
	if err != nil {
		t.Fatalf("GetLatestByTarget() err = %v", err)
	}
	if latest.ID != "diag-z" {
		t.Fatalf("expected latest id diag-z, got %s", latest.ID)
	}
}

func TestRepositorySetCancelledOnlyRunning(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	target := domain.Target{Cluster: "c1", ClusterDisplay: "C1", Namespace: "ns", Pod: "pod1"}

	if err := repo.Create(ctx, "d1", target); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetCancelled(ctx, "d1"); err != nil {
		t.Fatalf("SetCancelled() err = %v", err)
	}
	d, err := repo.GetByID(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.StatusCancelled {
		t.Fatalf("expected cancelled, got %s", d.Status)
	}

	if err := repo.SetCompleted(ctx, "d1", &domain.Result{
		Verdict:   domain.VerdictConfirmed,
		RootCause: domain.RootCause{Summary: "done"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetCancelled(ctx, "d1"); err == nil {
		t.Fatal("expected SetCancelled to fail for completed diagnosis")
	}
}
