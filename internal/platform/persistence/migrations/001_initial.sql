CREATE TABLE IF NOT EXISTS diagnoses (
    id          TEXT PRIMARY KEY,
    cluster_fingerprint TEXT NOT NULL,
    cluster_display     TEXT NOT NULL,
    namespace   TEXT NOT NULL,
    pod         TEXT NOT NULL,
    pod_uid     TEXT,
    symptom_hash TEXT,

    root_cause  TEXT,
    confidence  TEXT,
    evidence    TEXT,
    fix_actions TEXT,
    impact      TEXT,
    duration_ms INTEGER,

    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    resolved    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS activities (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    text        TEXT NOT NULL,
    cluster_display TEXT,
    diagnosis_id TEXT,
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_diagnoses_cluster ON diagnoses(cluster_fingerprint);
CREATE INDEX IF NOT EXISTS idx_diagnoses_created ON diagnoses(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activities_created ON activities(created_at DESC);
