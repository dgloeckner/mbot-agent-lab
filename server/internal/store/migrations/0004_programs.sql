-- Team tools ("eigene Werkzeuge"): the working state a team edits in a task.
-- The browser sends the Blockly state; the server derives the tree and the
-- problems from it and stores all three, also when the tool is invalid.
-- Saving is "last write wins": no version column, no lock.
CREATE TABLE programs (
    id          TEXT PRIMARY KEY,           -- stable id, "p" + random
    team_id     TEXT NOT NULL,
    task_id     TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    blockly     TEXT NOT NULL,              -- Blockly workspace state, JSON
    tree        TEXT NOT NULL,              -- derived program tree, JSON
    problems    TEXT NOT NULL DEFAULT '[]', -- derived problems, JSON; empty: valid
    updated_at  TEXT NOT NULL,
    UNIQUE (team_id, task_id, name),
    FOREIGN KEY (team_id, task_id) REFERENCES team_tasks (team_id, task_id)
);
