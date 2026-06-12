package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/audit/domain"
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
		INSERT INTO audits (id, cluster_fingerprint, cluster_display, status, created_at)
		VALUES (?, ?, ?, 'running', ?)`,
		id, target.Cluster, target.ClusterDisplay, now,
	)
	if err != nil {
		return fmt.Errorf("create audit: %w", err)
	}
	return nil
}

func (r *Repository) AppendEvent(ctx context.Context, auditID string, ev domain.EventAppend) error {
	var seqNum int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq_num), 0) + 1 FROM audit_events WHERE audit_id=?`, auditID,
	).Scan(&seqNum); err != nil {
		return fmt.Errorf("next seq_num: %w", err)
	}

	now := time.Now().UnixMilli()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_events (
			audit_id, seq_num, event_type, message, summary, detail,
			payload_kind, payload_json, elapsed_ms, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		auditID, seqNum, ev.EventType, ev.Message, ev.Summary, ev.Detail,
		ev.PayloadKind, ev.PayloadJSON, ev.ElapsedMs, now,
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func (r *Repository) SetCompleted(ctx context.Context, auditID string, result *domain.Result) error {
	findingsJSON, _ := json.Marshal(result.Findings)
	summaryJSON, _ := json.Marshal(result.Summary)
	_, err := r.db.ExecContext(ctx, `
		UPDATE audits
		SET status='completed', findings_json=?, summary_json=?, markdown=?, duration_ms=?
		WHERE id=?`,
		string(findingsJSON), string(summaryJSON), result.Markdown, result.DurationMs, auditID,
	)
	if err != nil {
		return fmt.Errorf("set audit completed: %w", err)
	}
	return nil
}

func (r *Repository) SetFailed(ctx context.Context, auditID, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE audits SET status='failed', error_message=? WHERE id=?`, errMsg, auditID,
	)
	if err != nil {
		return fmt.Errorf("set audit failed: %w", err)
	}
	return nil
}

func (r *Repository) SetCancelled(ctx context.Context, auditID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE audits SET status='cancelled' WHERE id=? AND status='running'`, auditID,
	)
	if err != nil {
		return fmt.Errorf("set audit cancelled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set audit cancelled rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, auditID string) (*domain.Audit, error) {
	var a domain.Audit
	var findingsJSON, summaryJSON, markdown, errMsg sql.NullString
	var durationMs sql.NullInt64
	var unixTs int64

	err := r.db.QueryRowContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, status,
		       findings_json, summary_json, markdown, error_message, duration_ms, created_at
		FROM audits WHERE id=?`, auditID,
	).Scan(&a.ID, &a.ClusterFingerprint, &a.ClusterDisplay, &a.Status,
		&findingsJSON, &summaryJSON, &markdown, &errMsg, &durationMs, &unixTs,
	)
	if err != nil {
		return nil, err
	}

	populateAuditResult(&a, findingsJSON, summaryJSON, markdown, errMsg, durationMs, unixTs)
	return &a, nil
}

func (r *Repository) GetLatestByCluster(ctx context.Context, cluster string) (*domain.Audit, error) {
	var a domain.Audit
	var findingsJSON, summaryJSON, markdown, errMsg sql.NullString
	var durationMs sql.NullInt64
	var unixTs int64

	err := r.db.QueryRowContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, status,
		       findings_json, summary_json, markdown, error_message, duration_ms, created_at
		FROM audits
		WHERE cluster_fingerprint=?
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, cluster,
	).Scan(&a.ID, &a.ClusterFingerprint, &a.ClusterDisplay, &a.Status,
		&findingsJSON, &summaryJSON, &markdown, &errMsg, &durationMs, &unixTs,
	)
	if err != nil {
		return nil, err
	}

	populateAuditResult(&a, findingsJSON, summaryJSON, markdown, errMsg, durationMs, unixTs)
	return &a, nil
}

func populateAuditResult(
	a *domain.Audit,
	findingsJSON, summaryJSON, markdown, errMsg sql.NullString,
	durationMs sql.NullInt64,
	unixTs int64,
) {
	a.ErrorMessage = errMsg.String
	a.DurationMs = durationMs.Int64
	a.CreatedAt = time.Unix(unixTs, 0)

	if a.Status == domain.StatusCompleted {
		result := &domain.Result{DurationMs: a.DurationMs}
		if markdown.Valid {
			result.Markdown = markdown.String
		}
		if findingsJSON.Valid && findingsJSON.String != "" {
			_ = json.Unmarshal([]byte(findingsJSON.String), &result.Findings)
		}
		if summaryJSON.Valid && summaryJSON.String != "" {
			_ = json.Unmarshal([]byte(summaryJSON.String), &result.Summary)
		}
		a.Result = result
	}
}

func (r *Repository) ListEventsSince(ctx context.Context, auditID string, sinceSeqNum int) ([]domain.EventRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, audit_id, seq_num, event_type, message, summary, detail,
		       payload_kind, payload_json, elapsed_ms, created_at
		FROM audit_events
		WHERE audit_id=? AND seq_num > ?
		ORDER BY seq_num ASC
		LIMIT 500`, auditID, sinceSeqNum,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()

	var events []domain.EventRecord
	for rows.Next() {
		var e domain.EventRecord
		if err := rows.Scan(&e.ID, &e.AuditID, &e.SeqNum, &e.EventType, &e.Message,
			&e.Summary, &e.Detail, &e.PayloadKind, &e.PayloadJSON,
			&e.ElapsedMs, &e.CreatedAt); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]domain.Audit, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audits`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audits: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, cluster_fingerprint, cluster_display, status, created_at
		FROM audits ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list audits: %w", err)
	}
	defer rows.Close()

	var list []domain.Audit
	for rows.Next() {
		var a domain.Audit
		var unixTs int64
		if err := rows.Scan(&a.ID, &a.ClusterFingerprint, &a.ClusterDisplay, &a.Status, &unixTs); err != nil {
			continue
		}
		a.CreatedAt = time.Unix(unixTs, 0)
		list = append(list, a)
	}
	return list, total, rows.Err()
}
