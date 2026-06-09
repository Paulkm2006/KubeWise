package handler

import (
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

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
