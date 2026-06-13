-- Create event log table
-- Note: ALTER TABLE diagnoses ADD COLUMN status is handled in Go code
-- (ensureDiagnosisStatusColumn) because SQLite doesn't support IF NOT EXISTS

CREATE TABLE IF NOT EXISTS diagnosis_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    diagnosis_id TEXT NOT NULL REFERENCES diagnoses(id) ON DELETE CASCADE,
    seq_num      INTEGER NOT NULL,
    event_type   TEXT NOT NULL,
    message      TEXT DEFAULT '',
    detail       TEXT DEFAULT '',
    token_in     INTEGER DEFAULT 0,
    token_out    INTEGER DEFAULT 0,
    elapsed_ms   INTEGER DEFAULT 0,
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    UNIQUE(diagnosis_id, seq_num)
);

CREATE INDEX IF NOT EXISTS idx_events_diag_seq
    ON diagnosis_events(diagnosis_id, seq_num);