package diagnosis

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/db"
	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

// Runner manages diagnosis lifecycle via DB operations.
// No in-memory buffer — all state lives in SQLite.
type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Start(ctx context.Context, id string, target DiagnosisTarget) error {
	now := time.Now().Unix()
	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO diagnoses (id, cluster_fingerprint, cluster_display, namespace, pod, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'running', ?)`,
		id, target.Cluster, target.ClusterDisplay, target.Namespace, target.Pod, now,
	)
	if err != nil {
		log.Ctx(ctx).Error("runner: failed to start diagnosis", zap.String("diagnosis_id", id), zap.Error(err))
		return err
	}
	log.Ctx(ctx).Info("runner: diagnosis started", zap.String("diagnosis_id", id))
	return nil
}

// PushEvent maps an agent event.Event to diagnosis_events row and inserts it.
func (r *Runner) PushEvent(ctx context.Context, diagID string, ev event.Event) error {
	// Determine next seq_num
	var seqNum int
	err := db.DB.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq_num), 0) + 1 FROM diagnosis_events WHERE diagnosis_id=?`, diagID,
	).Scan(&seqNum)
	if err != nil {
		return err
	}

	var eventType, message, detail string
	var tokenIn, tokenOut, elapsedMs int

	switch e := ev.(type) {
	case event.Phase:
		eventType, message = "phase", e.Phase
	case event.AgentStart:
		eventType, message = "agent_start", e.AgentName
	case event.AgentDone:
		eventType, detail = "agent_done", e.Result
		tokenIn, tokenOut = e.InTokens, e.OutTokens
		elapsedMs = int(e.Duration.Milliseconds())
	case event.ToolCall:
		eventType, message = "tool_call", e.ToolName
	case event.ToolDone:
		eventType, message = "tool_done", e.ToolName
		elapsedMs = int(e.Elapsed.Milliseconds())
	case event.ToolFail:
		eventType, message, detail = "tool_fail", e.ToolName, e.Err
	case event.LLMTextDelta:
		eventType, message = "llm_text_delta", e.Delta
	case event.Supervisor:
		eventType, message, detail = "supervisor", e.Decision, e.Detail
	case event.StreamDone:
		eventType = "stream_done"
	case event.StreamErr:
		eventType = "stream_err"
		if e.Err != nil {
			detail = e.Err.Error()
		}
	default:
		// Skip unhandled event types (InteractionRequest, etc.)
		return nil
	}

	return r.insertEvent(ctx, diagID, seqNum, eventType, message, detail, tokenIn, tokenOut, elapsedMs)
}

func (r *Runner) insertEvent(ctx context.Context, diagID string, seqNum int, eventType, message, detail string, tokenIn, tokenOut, elapsedMs int) error {
	now := time.Now().Unix()
	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO diagnosis_events (diagnosis_id, seq_num, event_type, message, detail, token_in, token_out, elapsed_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		diagID, seqNum, eventType, message, detail, tokenIn, tokenOut, elapsedMs, now,
	)
	if err != nil {
		log.Ctx(ctx).Warn("runner: failed to push event",
			zap.String("diagnosis_id", diagID),
			zap.String("event_type", eventType),
			zap.Error(err),
		)
	}
	return err
}

func (r *Runner) SetCompleted(ctx context.Context, diagID string, result *DiagnosisResult) error {
	evidenceJSON, _ := json.Marshal(result.Evidence)
	fixJSON, _ := json.Marshal(result.FixActions)

	_, err := db.DB.ExecContext(ctx, `
		UPDATE diagnoses
		SET status='completed', root_cause=?, confidence=?, evidence=?, fix_actions=?, impact=?, duration_ms=?
		WHERE id=?`,
		result.RootCause, result.Confidence, string(evidenceJSON), string(fixJSON),
		result.Impact, result.DurationMs, diagID,
	)
	if err != nil {
		log.Ctx(ctx).Error("runner: failed to set completed", zap.String("diagnosis_id", diagID), zap.Error(err))
	}
	return err
}

func (r *Runner) SetFailed(ctx context.Context, diagID string, errMsg string) error {
	_, err := db.DB.ExecContext(ctx, `UPDATE diagnoses SET status='failed', root_cause=? WHERE id=?`, errMsg, diagID)
	if err != nil {
		log.Ctx(ctx).Error("runner: failed to set failed", zap.String("diagnosis_id", diagID), zap.Error(err))
	}
	return err
}

// GetStatus returns the full Diagnosis record from DB.
func (r *Runner) GetStatus(ctx context.Context, diagID string) (*Diagnosis, error) {
	var d Diagnosis
	var evidenceJSON, fixJSON sql.NullString
	var rootCause, confidence, impact sql.NullString
	var durationMs sql.NullInt64
	var unixTs int64

	err := db.DB.QueryRowContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, namespace, pod,
		       status, root_cause, confidence, evidence, fix_actions, impact, duration_ms, created_at
		FROM diagnoses WHERE id=?`, diagID,
	).Scan(&d.ID, &d.ClusterFingerprint, &d.ClusterDisplay, &d.Namespace, &d.Pod,
		&d.Status, &rootCause, &confidence, &evidenceJSON, &fixJSON, &impact, &durationMs, &unixTs,
	)
	if err != nil {
		return nil, err
	}

	d.RootCause = rootCause.String
	d.Confidence = confidence.String
	d.Impact = impact.String
	d.DurationMs = durationMs.Int64
	d.CreatedAt = time.Unix(unixTs, 0)

	if evidenceJSON.Valid && evidenceJSON.String != "" {
		json.Unmarshal([]byte(evidenceJSON.String), &d.Evidence)
	}
	if fixJSON.Valid && fixJSON.String != "" {
		json.Unmarshal([]byte(fixJSON.String), &d.FixActions)
	}

	return &d, nil
}

// GetEventsSince returns all events with seq_num > since, ordered by seq_num, capped at 500.
func (r *Runner) GetEventsSince(ctx context.Context, diagID string, sinceSeqNum int) ([]EventRecord, error) {
	rows, err := db.DB.QueryContext(ctx, `
		SELECT id, diagnosis_id, seq_num, event_type, message, detail,
		       token_in, token_out, elapsed_ms, created_at
		FROM diagnosis_events
		WHERE diagnosis_id=? AND seq_num > ?
		ORDER BY seq_num ASC
		LIMIT 500`, diagID, sinceSeqNum,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventRecord
	for rows.Next() {
		var e EventRecord
		if err := rows.Scan(&e.ID, &e.DiagnosisID, &e.SeqNum, &e.EventType, &e.Message,
			&e.Detail, &e.TokenIn, &e.TokenOut, &e.ElapsedMs, &e.CreatedAt); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// List returns all diagnoses ordered by created_at DESC, paginated.
func (r *Runner) List(ctx context.Context, limit, offset int) ([]Diagnosis, int, error) {
	var total int
	db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM diagnoses`).Scan(&total)

	rows, err := db.DB.QueryContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, namespace, pod, status, created_at
		FROM diagnoses ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []Diagnosis
	for rows.Next() {
		var d Diagnosis
		var unixTs int64
		if err := rows.Scan(&d.ID, &d.ClusterFingerprint, &d.ClusterDisplay,
			&d.Namespace, &d.Pod, &d.Status, &unixTs); err != nil {
			continue
		}
		d.CreatedAt = time.Unix(unixTs, 0)
		list = append(list, d)
	}
	return list, total, rows.Err()
}