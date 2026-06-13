CREATE TABLE IF NOT EXISTS audits (
    id                  TEXT PRIMARY KEY,
    cluster_fingerprint TEXT NOT NULL,
    cluster_display     TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'running',
    findings_json       TEXT,
    summary_json        TEXT,
    markdown            TEXT,
    error_message       TEXT,
    duration_ms         INTEGER,
    created_at          INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE TABLE IF NOT EXISTS audit_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    audit_id     TEXT NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
    seq_num      INTEGER NOT NULL,
    event_type   TEXT NOT NULL,
    message      TEXT DEFAULT '',
    summary      TEXT DEFAULT '',
    detail       TEXT DEFAULT '',
    payload_kind TEXT DEFAULT '',
    payload_json TEXT DEFAULT '',
    elapsed_ms   INTEGER DEFAULT 0,
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(audit_id, seq_num)
);

CREATE INDEX IF NOT EXISTS idx_audits_cluster ON audits(cluster_fingerprint, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_seq ON audit_events(audit_id, seq_num);
