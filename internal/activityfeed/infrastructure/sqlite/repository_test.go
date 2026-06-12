package sqlite

import (
	"context"
	"testing"

	"github.com/kubewise/kubewise/internal/activityfeed/domain"
)

func TestRepositoryInsertAndList(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	a, err := repo.Insert(ctx, domain.TypeDiagnosis, "nginx OOMKilled", "prod-us", "diag-1")
	if err != nil {
		t.Fatalf("Insert() err = %v", err)
	}
	if a.Type != domain.TypeDiagnosis {
		t.Fatalf("expected TypeDiagnosis, got %v", a.Type)
	}

	items, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() err = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(items))
	}
}
