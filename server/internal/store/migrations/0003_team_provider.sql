-- The model provider a team chose for its runs (a name from llm.providers in
-- the lab configuration). Empty means the lab's default provider.
ALTER TABLE teams ADD COLUMN provider TEXT NOT NULL DEFAULT '';
