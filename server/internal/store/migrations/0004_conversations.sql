-- Conversations: while no run is active, a team talks with its agent. A
-- conversation is a row in runs with kind 'conversation', so its messages,
-- tool calls and model calls use the same tables as a run. Everything that
-- means "runs" filters on kind = 'run'.
ALTER TABLE runs ADD COLUMN kind TEXT NOT NULL DEFAULT 'run';
CREATE INDEX runs_team_kind ON runs (team_id, kind, id);
