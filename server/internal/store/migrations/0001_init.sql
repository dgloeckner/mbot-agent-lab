-- Initial schema, see docs/handoff.md "Datenmodell in SQLite".
-- Timestamps are RFC 3339 strings in UTC.

CREATE TABLE prompts (
    id         INTEGER PRIMARY KEY,
    team_id    TEXT    NOT NULL,
    version    INTEGER NOT NULL,
    text       TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    UNIQUE (team_id, version)
);

CREATE TABLE runs (
    id             INTEGER PRIMARY KEY,
    team_id        TEXT    NOT NULL,
    prompt_id      INTEGER REFERENCES prompts (id),
    model          TEXT    NOT NULL,
    started_at     TEXT    NOT NULL,
    ended_at       TEXT,
    end_reason     TEXT,
    tokens_in      INTEGER NOT NULL DEFAULT 0,
    tokens_out     INTEGER NOT NULL DEFAULT 0,
    cost_microeur  INTEGER NOT NULL DEFAULT 0,
    report         TEXT
);
CREATE INDEX runs_team ON runs (team_id, started_at);

CREATE TABLE messages (
    id      INTEGER PRIMARY KEY,
    run_id  INTEGER NOT NULL REFERENCES runs (id),
    seq     INTEGER NOT NULL,
    source  TEXT    NOT NULL, -- robot | team | agent:<name> | system
    kind    TEXT    NOT NULL, -- event | instruction | chat | deploy | control
    text    TEXT    NOT NULL DEFAULT '',
    data    TEXT,             -- JSON
    at      TEXT    NOT NULL,
    UNIQUE (run_id, seq)
);

CREATE TABLE tool_calls (
    id          INTEGER PRIMARY KEY,
    run_id      INTEGER NOT NULL REFERENCES runs (id),
    seq         INTEGER NOT NULL,
    name        TEXT    NOT NULL,
    args        TEXT    NOT NULL, -- JSON
    result      TEXT,             -- JSON
    duration_ms INTEGER,
    at          TEXT    NOT NULL,
    UNIQUE (run_id, seq)
);

CREATE TABLE llm_calls (
    id            INTEGER PRIMARY KEY,
    run_id        INTEGER NOT NULL REFERENCES runs (id),
    provider      TEXT    NOT NULL,
    model         TEXT    NOT NULL,
    tokens_in     INTEGER NOT NULL,
    tokens_out    INTEGER NOT NULL,
    duration_ms   INTEGER NOT NULL,
    cost_microeur INTEGER NOT NULL DEFAULT 0,
    at            TEXT    NOT NULL
);

CREATE TABLE notes (
    id             INTEGER PRIMARY KEY,
    team_id        TEXT    NOT NULL,
    text           TEXT    NOT NULL,
    origin_run_id  INTEGER REFERENCES runs (id),
    origin_msg_id  INTEGER REFERENCES messages (id), -- message that led to the note
    author         TEXT    NOT NULL, -- agent | team
    created_at     TEXT    NOT NULL
);
CREATE INDEX notes_team ON notes (team_id);

-- One row per robot: the exclusivity rule is enforced by the primary key.
CREATE TABLE assignments (
    robot_id   TEXT PRIMARY KEY,
    owner      TEXT NOT NULL, -- team:<id> | mcp:<connection>
    alias      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
