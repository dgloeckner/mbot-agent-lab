-- The event log: coarse facts that can count for a result, at the finest
-- scope known. Kinds and their data fields: docs/events.md. Scope columns are
-- real columns, so "per (team, task, robot, agent version)" is a plain
-- GROUP BY. SQLite skips the composite foreign key when one of its columns is
-- NULL, so team-only events (team.joined) work.
CREATE TABLE events (
    id        INTEGER PRIMARY KEY,
    at        TEXT NOT NULL,
    kind      TEXT NOT NULL,
    actor     TEXT NOT NULL,              -- admin | team | agent | robot | system
    team_id   TEXT REFERENCES teams (id),
    task_id   TEXT REFERENCES tasks (id),
    robot_id  TEXT REFERENCES robots (id),
    prompt_id INTEGER REFERENCES prompts (id),
    run_id    INTEGER REFERENCES runs (id),
    data      TEXT,                       -- JSON, small
    FOREIGN KEY (team_id, task_id) REFERENCES team_tasks (team_id, task_id)
);
CREATE INDEX events_scope ON events (task_id, team_id, robot_id, prompt_id);
