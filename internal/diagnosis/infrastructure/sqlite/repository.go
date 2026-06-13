package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/diagnosis/domain"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, id string, target domain.Target) error {
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO diagnoses (id, cluster_fingerprint, cluster_display, namespace, pod, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'running', ?)`,
		id, target.Cluster, target.ClusterDisplay, target.Namespace, target.Pod, now,
	)
	if err != nil {
		return fmt.Errorf("create diagnosis: %w", err)
	}
	return nil
}

func (r *Repository) AppendEvent(ctx context.Context, diagID string, ev domain.EventAppend) error {
	var seqNum int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq_num), 0) + 1 FROM diagnosis_events WHERE diagnosis_id=?`, diagID,
	).Scan(&seqNum); err != nil {
		return fmt.Errorf("next seq_num: %w", err)
	}

	now := time.Now().UnixMilli()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO diagnosis_events (
			diagnosis_id, seq_num, event_type, message, summary, detail,
			payload_kind, payload_json, token_in, token_out, elapsed_ms, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		diagID, seqNum, ev.EventType, ev.Message, ev.Summary, ev.Detail,
		ev.PayloadKind, ev.PayloadJSON, ev.TokenIn, ev.TokenOut, ev.ElapsedMs, now,
	)
	if err != nil {
		return fmt.Errorf("insert diagnosis event: %w", err)
	}
	return nil
}

func (r *Repository) SetCompleted(ctx context.Context, diagID string, result *domain.Result) error {
	reportJSON, _ := json.Marshal(result)
	evidenceJSON, _ := json.Marshal(result.Evidence)
	fixJSON, _ := json.Marshal(result.Actions)
	rootSummary := result.RootCause.Summary
	if rootSummary == "" {
		rootSummary = result.RootCause.Title
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE diagnoses
		SET status='completed', report_json=?, root_cause=?, confidence=?, evidence=?, fix_actions=?, impact=?, duration_ms=?
		WHERE id=?`,
		string(reportJSON), rootSummary, result.RootCause.ConfidenceLabel,
		string(evidenceJSON), string(fixJSON), result.Impact.Description,
		result.DurationMs, diagID,
	)
	if err != nil {
		return fmt.Errorf("set diagnosis completed: %w", err)
	}
	return nil
}

func (r *Repository) SetFailed(ctx context.Context, diagID, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE diagnoses SET status='failed', root_cause=? WHERE id=?`, errMsg, diagID)
	if err != nil {
		return fmt.Errorf("set diagnosis failed: %w", err)
	}
	return nil
}

func (r *Repository) SetCancelled(ctx context.Context, diagID string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE diagnoses SET status='cancelled' WHERE id=? AND status='running'`, diagID)
	if err != nil {
		return fmt.Errorf("set diagnosis cancelled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set diagnosis cancelled rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetLatestByTarget(ctx context.Context, target domain.Target) (*domain.Diagnosis, error) {
	var d domain.Diagnosis
	var evidenceJSON, fixJSON, reportJSON sql.NullString
	var rootCause, confidence, impact sql.NullString
	var durationMs sql.NullInt64
	var unixTs int64

	err := r.db.QueryRowContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, namespace, pod,
		       status, root_cause, confidence, evidence, fix_actions, impact, duration_ms, report_json, created_at
		FROM diagnoses
		WHERE cluster_fingerprint=? AND namespace=? AND pod=?
		ORDER BY created_at DESC, id DESC
		LIMIT 1`,
		target.Cluster, target.Namespace, target.Pod,
	).Scan(&d.ID, &d.ClusterFingerprint, &d.ClusterDisplay, &d.Namespace, &d.Pod,
		&d.Status, &rootCause, &confidence, &evidenceJSON, &fixJSON, &impact, &durationMs, &reportJSON, &unixTs,
	)
	if err != nil {
		return nil, err
	}

	populateDiagnosisFields(&d, rootCause, confidence, impact, durationMs, evidenceJSON, fixJSON, reportJSON, unixTs)
	return &d, nil
}

func (r *Repository) GetByID(ctx context.Context, diagID string) (*domain.Diagnosis, error) {
	var d domain.Diagnosis
	var evidenceJSON, fixJSON, reportJSON sql.NullString
	var rootCause, confidence, impact sql.NullString
	var durationMs sql.NullInt64
	var unixTs int64

	err := r.db.QueryRowContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, namespace, pod,
		       status, root_cause, confidence, evidence, fix_actions, impact, duration_ms, report_json, created_at
		FROM diagnoses WHERE id=?`, diagID,
	).Scan(&d.ID, &d.ClusterFingerprint, &d.ClusterDisplay, &d.Namespace, &d.Pod,
		&d.Status, &rootCause, &confidence, &evidenceJSON, &fixJSON, &impact, &durationMs, &reportJSON, &unixTs,
	)
	if err != nil {
		return nil, err
	}

	populateDiagnosisFields(&d, rootCause, confidence, impact, durationMs, evidenceJSON, fixJSON, reportJSON, unixTs)
	return &d, nil
}

func populateDiagnosisFields(
	d *domain.Diagnosis,
	rootCause, confidence, impact sql.NullString,
	durationMs sql.NullInt64,
	evidenceJSON, fixJSON, reportJSON sql.NullString,
	unixTs int64,
) {
	d.RootCause = rootCause.String
	d.Confidence = confidence.String
	d.Impact = impact.String
	d.DurationMs = durationMs.Int64
	d.CreatedAt = time.Unix(unixTs, 0)

	if reportJSON.Valid && reportJSON.String != "" {
		var rep domain.Result
		if err := json.Unmarshal([]byte(reportJSON.String), &rep); err == nil {
			d.Report = &rep
		}
	}
	if d.Report == nil && d.RootCause != "" {
		d.Report = &domain.Result{
			Verdict: domain.VerdictInconclusive,
			RootCause: domain.RootCause{
				Title: d.RootCause, Summary: d.RootCause, ConfidenceLabel: d.Confidence,
			},
			Impact: domain.Impact{Description: d.Impact},
		}
	}
	if evidenceJSON.Valid && evidenceJSON.String != "" {
		_ = json.Unmarshal([]byte(evidenceJSON.String), &d.Evidence)
	}
	if fixJSON.Valid && fixJSON.String != "" {
		_ = json.Unmarshal([]byte(fixJSON.String), &d.FixActions)
	}
	if d.Report != nil {
		if len(d.Evidence) == 0 {
			d.Evidence = d.Report.Evidence
		}
		if len(d.FixActions) == 0 {
			d.FixActions = d.Report.Actions
		}
	}
}

func (r *Repository) ListEventsSince(ctx context.Context, diagID string, sinceSeqNum int) ([]domain.EventRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, diagnosis_id, seq_num, event_type, message, summary, detail,
		       payload_kind, payload_json, token_in, token_out, elapsed_ms, created_at
		FROM diagnosis_events
		WHERE diagnosis_id=? AND seq_num > ?
		ORDER BY seq_num ASC
		LIMIT 500`, diagID, sinceSeqNum,
	)
	if err != nil {
		return nil, fmt.Errorf("list diagnosis events: %w", err)
	}
	defer rows.Close()

	var events []domain.EventRecord
	for rows.Next() {
		var e domain.EventRecord
		if err := rows.Scan(&e.ID, &e.DiagnosisID, &e.SeqNum, &e.EventType, &e.Message,
			&e.Summary, &e.Detail, &e.PayloadKind, &e.PayloadJSON,
			&e.TokenIn, &e.TokenOut, &e.ElapsedMs, &e.CreatedAt); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]domain.Diagnosis, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM diagnoses`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count diagnoses: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, namespace, pod, status, created_at
		FROM diagnoses ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list diagnoses: %w", err)
	}
	defer rows.Close()

	var list []domain.Diagnosis
	for rows.Next() {
		var d domain.Diagnosis
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
