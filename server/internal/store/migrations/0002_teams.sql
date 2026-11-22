-- Teams join with the lab code and are managed in the control room instead of
-- the YAML configuration. Robot assignments use the existing assignments table
-- (one row per robot, so a robot can never have two owners).

CREATE TABLE teams (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE, -- secret part of the team link
    color      TEXT NOT NULL,        -- page accent and LED color of the team's robots
    created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX teams_name ON teams (name COLLATE NOCASE);

-- A team cannot give two of its robots the same name.
CREATE UNIQUE INDEX assignments_alias ON assignments (owner, alias COLLATE NOCASE);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
