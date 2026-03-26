package db

// initSchema crea todas las tablas, índices, triggers y views.
// Es un método del struct DB (receptor *DB).
// Se llama desde OpenAt() al abrir la base de datos.
func (db *DB) initSchema() error {
	_, err := db.conn.Exec(schema)
	return err
}

// schema es una constante con todo el DDL.
// Las constantes a nivel de paquete se evalúan en compilación.
// Usamos backticks (raw string) para multi-línea.
const schema = `

-- ═══════════════════════════════════════════
-- 1. PROJECTS
-- ═══════════════════════════════════════════
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    path        TEXT,
    git_remote  TEXT,
    workspace   TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_projects_workspace ON projects(workspace);

-- ═══════════════════════════════════════════
-- 2. SESSIONS
-- ═══════════════════════════════════════════
CREATE TABLE IF NOT EXISTS sessions (
    id                TEXT PRIMARY KEY,
    project_id        TEXT,
    started_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ended_at          TEXT,
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK(status IN ('active', 'completed', 'compacted', 'abandoned')),
    objectives        TEXT DEFAULT '[]',
    summary           TEXT,
    tasks_completed   TEXT DEFAULT '[]',
    files_touched     TEXT DEFAULT '[]',
    compaction_count  INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status  ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at DESC);

-- ═══════════════════════════════════════════
-- 3. OBSERVATIONS
-- ═══════════════════════════════════════════
CREATE TABLE IF NOT EXISTS observations (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL,
    project_id        TEXT,
    scope             TEXT NOT NULL DEFAULT 'project'
                      CHECK(scope IN ('global', 'project', 'workspace')),
    category          TEXT NOT NULL
                      CHECK(category IN ('decision', 'bug', 'pattern', 'note', 'request', 'preference')),
    title             TEXT NOT NULL,
    content           TEXT NOT NULL,
    tags              TEXT DEFAULT '[]',
    files             TEXT DEFAULT '[]',
    relevance_score   REAL NOT NULL DEFAULT 1.0,
    access_count      INTEGER NOT NULL DEFAULT 0,
    last_accessed_at  TEXT,
    superseded_by     INTEGER,
    is_active         INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    FOREIGN KEY (session_id)    REFERENCES sessions(id),
    FOREIGN KEY (project_id)    REFERENCES projects(id),
    FOREIGN KEY (superseded_by) REFERENCES observations(id)
);

CREATE INDEX IF NOT EXISTS idx_obs_session   ON observations(session_id);
CREATE INDEX IF NOT EXISTS idx_obs_project   ON observations(project_id);
CREATE INDEX IF NOT EXISTS idx_obs_scope     ON observations(scope);
CREATE INDEX IF NOT EXISTS idx_obs_category  ON observations(category);
CREATE INDEX IF NOT EXISTS idx_obs_active    ON observations(is_active);
CREATE INDEX IF NOT EXISTS idx_obs_relevance ON observations(relevance_score DESC);
CREATE INDEX IF NOT EXISTS idx_obs_created   ON observations(created_at DESC);

-- ═══════════════════════════════════════════
-- 4. FTS5: Full-Text Search
-- ═══════════════════════════════════════════
CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    title,
    content,
    tags,
    content = observations,
    content_rowid = id,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS trg_obs_fts_insert
AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts(rowid, title, content, tags)
    VALUES (new.id, new.title, new.content, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS trg_obs_fts_delete
AFTER DELETE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, title, content, tags)
    VALUES ('delete', old.id, old.title, old.content, old.tags);
END;

CREATE TRIGGER IF NOT EXISTS trg_obs_fts_update
AFTER UPDATE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, title, content, tags)
    VALUES ('delete', old.id, old.title, old.content, old.tags);
    INSERT INTO observations_fts(rowid, title, content, tags)
    VALUES (new.id, new.title, new.content, new.tags);
END;

-- ═══════════════════════════════════════════
-- 5. RELATIONS
-- ═══════════════════════════════════════════
CREATE TABLE IF NOT EXISTS relations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id       INTEGER NOT NULL,
    target_id       INTEGER NOT NULL,
    relation_type   TEXT NOT NULL
                    CHECK(relation_type IN (
                        'caused_by', 'supersedes', 'relates_to',
                        'contradicts', 'depends_on', 'derived_from'
                    )),
    description     TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    FOREIGN KEY (source_id) REFERENCES observations(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES observations(id) ON DELETE CASCADE,
    UNIQUE(source_id, target_id, relation_type)
);

CREATE INDEX IF NOT EXISTS idx_rel_source ON relations(source_id);
CREATE INDEX IF NOT EXISTS idx_rel_target ON relations(target_id);
CREATE INDEX IF NOT EXISTS idx_rel_type   ON relations(relation_type);

-- ═══════════════════════════════════════════
-- 6. USER REQUESTS
-- ═══════════════════════════════════════════
CREATE TABLE IF NOT EXISTS user_requests (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      TEXT NOT NULL,
    project_id      TEXT,
    request         TEXT NOT NULL,
    priority        TEXT NOT NULL DEFAULT 'normal'
                    CHECK(priority IN ('low', 'normal', 'high', 'critical')),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK(status IN ('pending', 'in_progress', 'completed', 'deferred')),
    resolution      TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    completed_at    TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id),
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE VIRTUAL TABLE IF NOT EXISTS requests_fts USING fts5(
    request,
    resolution,
    content = user_requests,
    content_rowid = id,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS trg_req_fts_insert
AFTER INSERT ON user_requests BEGIN
    INSERT INTO requests_fts(rowid, request, resolution)
    VALUES (new.id, new.request, COALESCE(new.resolution, ''));
END;

CREATE TRIGGER IF NOT EXISTS trg_req_fts_update
AFTER UPDATE ON user_requests BEGIN
    INSERT INTO requests_fts(requests_fts, rowid, request, resolution)
    VALUES ('delete', old.id, old.request, COALESCE(old.resolution, ''));
    INSERT INTO requests_fts(rowid, request, resolution)
    VALUES (new.id, new.request, COALESCE(new.resolution, ''));
END;

CREATE INDEX IF NOT EXISTS idx_req_session  ON user_requests(session_id);
CREATE INDEX IF NOT EXISTS idx_req_project  ON user_requests(project_id);
CREATE INDEX IF NOT EXISTS idx_req_status   ON user_requests(status);
CREATE INDEX IF NOT EXISTS idx_req_priority ON user_requests(priority);

-- ═══════════════════════════════════════════
-- 7. COMPACTION SNAPSHOTS
-- ═══════════════════════════════════════════
CREATE TABLE IF NOT EXISTS compaction_snapshots (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id              TEXT NOT NULL,
    snapshot_type           TEXT NOT NULL
                            CHECK(snapshot_type IN ('pre_compact', 'session_end', 'manual')),
    summary                 TEXT NOT NULL,
    raw_length              INTEGER,
    observations_extracted  INTEGER NOT NULL DEFAULT 0,
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_snap_session ON compaction_snapshots(session_id);

-- ═══════════════════════════════════════════
-- 8. VIEWS
-- ═══════════════════════════════════════════
CREATE VIEW IF NOT EXISTS observations_ranked AS
SELECT
    o.*,
    o.relevance_score * POWER(0.99,
        JULIANDAY('now') - JULIANDAY(COALESCE(o.last_accessed_at, o.created_at))
    ) AS effective_score
FROM observations o
WHERE o.is_active = 1;

CREATE VIEW IF NOT EXISTS context_view AS
SELECT
    o.id, o.scope, o.category, o.title, o.content,
    o.tags, o.files, o.project_id,
    o.relevance_score * POWER(0.99,
        JULIANDAY('now') - JULIANDAY(COALESCE(o.last_accessed_at, o.created_at))
    ) AS effective_score,
    o.created_at, o.access_count
FROM observations o
WHERE o.is_active = 1
ORDER BY
    CASE o.category WHEN 'request' THEN 0 ELSE 1 END,
    effective_score DESC;
`