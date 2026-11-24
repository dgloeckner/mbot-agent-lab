-- Schema of the lab database, see docs/architecture.md "Data model".
-- Timestamps are RFC 3339 strings in UTC.
--
-- Everything a team produces belongs to a (team, task) pair: prompt versions,
-- notes, runs and conversations. Those tables carry team_id and task_id with
-- a composite foreign key to team_tasks, so the database refuses rows for a
-- pair that was never assigned.

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Teams join with the lab code. Deleting a team only sets deleted_at, so
-- results and history stay consistent; a deleted team's link stops working
-- and its name is free again.
CREATE TABLE teams (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE,     -- secret part of the team link
    color      TEXT NOT NULL,            -- page accent and LED color of the team's robots
    provider   TEXT NOT NULL DEFAULT '', -- chosen model provider, '' = lab default
    created_at TEXT NOT NULL,
    deleted_at TEXT
);
CREATE UNIQUE INDEX teams_name ON teams (name COLLATE NOCASE) WHERE deleted_at IS NULL;

-- Every robot the lab knows, so runs and events can reference it. The ID is
-- the device ID from the configuration and the heartbeat, stable across renames.
CREATE TABLE robots (
    id            TEXT PRIMARY KEY,
    first_seen_at TEXT NOT NULL
);

-- One row per robot: the exclusivity rule is enforced by the primary key.
CREATE TABLE assignments (
    robot_id   TEXT PRIMARY KEY,
    owner      TEXT NOT NULL, -- team:<id>
    alias      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
-- A team cannot give two of its robots the same name.
CREATE UNIQUE INDEX assignments_alias ON assignments (owner, alias COLLATE NOCASE);

-- Tasks are prepared by the control room and shared by all teams. The body is
-- Markdown for the team; it never goes into the agent context.
CREATE TABLE tasks (
    id           TEXT PRIMARY KEY,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    starter      TEXT NOT NULL DEFAULT '', -- optional starter prompt
    state        TEXT NOT NULL,            -- draft | open | closed
    created_at   TEXT NOT NULL,
    published_at TEXT,
    closed_at    TEXT
);

-- A task assigned to a team; accepted_at is set once the team accepts it.
CREATE TABLE team_tasks (
    team_id     TEXT NOT NULL REFERENCES teams (id),
    task_id     TEXT NOT NULL REFERENCES tasks (id),
    assigned_at TEXT NOT NULL,
    accepted_at TEXT,
    PRIMARY KEY (team_id, task_id)
);

-- The task a team works in. The server owns it, so all PCs of a team follow.
CREATE TABLE active_tasks (
    team_id TEXT PRIMARY KEY REFERENCES teams (id),
    task_id TEXT NOT NULL,
    FOREIGN KEY (team_id, task_id) REFERENCES team_tasks (team_id, task_id)
);

-- Agent versions: prompt versions are numbered per (team, task), from 1.
CREATE TABLE prompts (
    id         INTEGER PRIMARY KEY,
    team_id    TEXT    NOT NULL,
    task_id    TEXT    NOT NULL,
    version    INTEGER NOT NULL,
    text       TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    UNIQUE (team_id, task_id, version),
    FOREIGN KEY (team_id, task_id) REFERENCES team_tasks (team_id, task_id)
);

-- Runs and conversations. A conversation (kind 'conversation') is the talk
-- between a team and its agent while no run is active; its messages, tool
-- calls and model calls use the same tables as a run. Everything that means
-- "runs" filters on kind = 'run'. prompt_id is the agent version of the run.
CREATE TABLE runs (
    id            INTEGER PRIMARY KEY,
    team_id       TEXT    NOT NULL,
    task_id       TEXT    NOT NULL,
    kind          TEXT    NOT NULL DEFAULT 'run', -- run | conversation
    prompt_id     INTEGER REFERENCES prompts (id),
    model         TEXT    NOT NULL,
    started_at    TEXT    NOT NULL,
    ended_at      TEXT,
    end_reason    TEXT,
    tokens_in     INTEGER NOT NULL DEFAULT 0,
    tokens_out    INTEGER NOT NULL DEFAULT 0,
    cost_microeur INTEGER NOT NULL DEFAULT 0,
    report        TEXT,
    FOREIGN KEY (team_id, task_id) REFERENCES team_tasks (team_id, task_id)
);
CREATE INDEX runs_scope ON runs (team_id, task_id, kind, id);

-- robot_id and robot_alias (the robot's name at that moment; teams rename
-- robots) are set for robot messages and robot tools.
CREATE TABLE messages (
    id          INTEGER PRIMARY KEY,
    run_id      INTEGER NOT NULL REFERENCES runs (id),
    seq         INTEGER NOT NULL,
    source      TEXT    NOT NULL, -- robot | team | agent:<name> | system
    kind        TEXT    NOT NULL,
    text        TEXT    NOT NULL DEFAULT '',
    data        TEXT,             -- JSON
    robot_id    TEXT REFERENCES robots (id),
    robot_alias TEXT,
    at          TEXT    NOT NULL,
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
    robot_id    TEXT REFERENCES robots (id),
    robot_alias TEXT,
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
    id            INTEGER PRIMARY KEY,
    team_id       TEXT    NOT NULL,
    task_id       TEXT    NOT NULL,
    text          TEXT    NOT NULL,
    origin_run_id INTEGER REFERENCES runs (id),
    origin_msg_id INTEGER REFERENCES messages (id), -- message that led to the note
    author        TEXT    NOT NULL,                 -- agent | team
    created_at    TEXT    NOT NULL,
    FOREIGN KEY (team_id, task_id) REFERENCES team_tasks (team_id, task_id)
);
CREATE INDEX notes_scope ON notes (team_id, task_id);
