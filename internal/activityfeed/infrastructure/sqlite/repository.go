package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/activityfeed/domain"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, typ domain.Type, text, clusterDisplay, diagnosisID string) (*domain.Activity, error) {
	now := time.Now()
	a := &domain.Activity{
		ID:             uuid.New().String(),
		Type:           typ,
		Text:           text,
		ClusterDisplay: clusterDisplay,
		DiagnosisID:    diagnosisID,
		CreatedAt:      now,
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO activities (id, type, text, cluster_display, diagnosis_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Text, a.ClusterDisplay, a.DiagnosisID, now.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert activity: %w", err)
	}
	return a, nil
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]domain.Activity, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, text, cluster_display, diagnosis_id, created_at FROM activities ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var activities []domain.Activity
	for rows.Next() {
		var a domain.Activity
		var diagID, clusterDisp sql.NullString
		var unixTs int64
		if err := rows.Scan(&a.ID, &a.Type, &a.Text, &clusterDisp, &diagID, &unixTs); err != nil {
			continue
		}
		a.ClusterDisplay = clusterDisp.String
		a.DiagnosisID = diagID.String
		a.CreatedAt = time.Unix(unixTs, 0)
		activities = append(activities, a)
	}
	return activities, rows.Err()
}
