package handler

import (
	"github.com/kubewise/kubewise/internal/diagnosis"
	"time"
)

// --- Response types ---

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

type DiagnoseResponse struct {
	DiagnosisID string `json:"diagnosis_id"`
	Status      string `json:"status"`
}

type DiagnosisStatusResponse struct {
	DiagnosisID string                   `json:"diagnosis_id"`
	Status      string                   `json:"status"`
	Target      diagnosis.DiagnosisTarget `json:"target"`
	Events      []diagnosis.EventRecord   `json:"events"`
	Result      *diagnosis.DiagnosisResult `json:"result,omitempty"`
}

type DiagnosisListResponse struct {
	Diagnoses []diagnosis.Diagnosis `json:"diagnoses"`
	Total     int                   `json:"total"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
