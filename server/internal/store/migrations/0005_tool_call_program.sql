-- A tool call that a team tool made by hand names that team tool, so the
-- protocol of the manual row shows which steps belong to which run.
ALTER TABLE tool_calls ADD COLUMN program TEXT;
