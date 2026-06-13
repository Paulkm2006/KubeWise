package domain

import "time"

type Type string

const (
	TypeDiagnosis     Type = "diagnosis"
	TypeClusterSwitch Type = "cluster_switch"
	TypeSystem        Type = "system"
)

type Activity struct {
	ID             string    `json:"id"`
	Type           Type      `json:"type"`
	Text           string    `json:"text"`
	ClusterDisplay string    `json:"cluster_display,omitempty"`
	DiagnosisID    string    `json:"diagnosis_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
