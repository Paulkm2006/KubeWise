package activity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Add(ctx context.Context, typ Type, text, clusterDisplay, diagnosisID string) (*Activity, error) {
	now := time.Now()
	a := &Activity{
		ID:             uuid.New().String(),
		Type:           typ,
		Text:           text,
		ClusterDisplay: clusterDisplay,
		DiagnosisID:    diagnosisID,
		CreatedAt:      now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO activities (id, type, text, cluster_display, diagnosis_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Text, a.ClusterDisplay, a.DiagnosisID, now.Unix(),
	)
	if err != nil {
		log.Ctx(ctx).Error("activity: failed to insert",
			zap.String("event", "activity.error"),
			zap.String("activity_type", string(typ)),
			zap.Error(err),
		)
		return nil, fmt.Errorf("insert activity: %w", err)
	}

	log.Ctx(ctx).Info("activity recorded",
		zap.String("event", "activity.created"),
		zap.String("activity_id", a.ID),
		zap.String("activity_type", string(typ)),
	)
	return a, nil
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]Activity, error) {
	log.Ctx(ctx).Debug("activity: listing",
		zap.Int("limit", limit),
		zap.Int("offset", offset),
	)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, text, cluster_display, diagnosis_id, created_at FROM activities ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		log.Ctx(ctx).Error("activity: failed to list", zap.Error(err))
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		var diagID, clusterDisp sql.NullString
		var unixTs int64
		if err := rows.Scan(&a.ID, &a.Type, &a.Text, &clusterDisp, &diagID, &unixTs); err != nil {
			log.Ctx(ctx).Error("activity: row scan error", zap.Error(err))
			continue
		}
		a.ClusterDisplay = clusterDisp.String
		a.DiagnosisID = diagID.String
		a.CreatedAt = time.Unix(unixTs, 0)
		activities = append(activities, a)
	}
	return activities, rows.Err()
}
