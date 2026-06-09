package activity

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Add(typ Type, text, clusterDisplay, diagnosisID string) (*Activity, error) {
	now := time.Now()
	a := &Activity{
		ID:             uuid.New().String(),
		Type:           typ,
		Text:           text,
		ClusterDisplay: clusterDisplay,
		DiagnosisID:    diagnosisID,
		CreatedAt:      now,
	}
	_, err := s.db.Exec(
		`INSERT INTO activities (id, type, text, cluster_display, diagnosis_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Text, a.ClusterDisplay, a.DiagnosisID, now.Unix(),
	)
	return a, err
}

func (s *Service) List(limit, offset int) ([]Activity, error) {
	rows, err := s.db.Query(
		`SELECT id, type, text, cluster_display, diagnosis_id, created_at FROM activities ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		var diagID, clusterDisp sql.NullString
		var unixTs int64
		if err := rows.Scan(&a.ID, &a.Type, &a.Text, &clusterDisp, &diagID, &unixTs); err != nil {
			return nil, err
		}
		a.ClusterDisplay = clusterDisp.String
		a.DiagnosisID = diagID.String
		a.CreatedAt = time.Unix(unixTs, 0)
		activities = append(activities, a)
	}
	return activities, rows.Err()
}
