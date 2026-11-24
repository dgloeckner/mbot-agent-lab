-- Which tools the agent gets in a task: all (the default) or team_tools
-- (only the team's deployed own tools plus a few that move no robot). Set on
-- the draft, fixed when the task is published.
ALTER TABLE tasks ADD COLUMN agent_tools TEXT NOT NULL DEFAULT 'all';
