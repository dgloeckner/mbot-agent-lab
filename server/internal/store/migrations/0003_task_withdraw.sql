-- Withdrawing an assignment the team has not accepted keeps the row (events
-- refer to the pair) and marks it withdrawn. Assigning again clears the mark.
ALTER TABLE team_tasks ADD COLUMN withdrawn_at TEXT;
