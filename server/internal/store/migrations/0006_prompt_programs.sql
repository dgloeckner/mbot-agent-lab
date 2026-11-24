-- One deploy is one version of prompt and team tools: every team tool of
-- the pair, valid or not, is frozen with the prompt version (prompts.id).
-- "Zurück" restores them, with their ids, into the working state.
CREATE TABLE prompt_programs (
    prompt_id   INTEGER NOT NULL REFERENCES prompts (id),
    program_id  TEXT    NOT NULL, -- the team tool's id at deploy time
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL,
    blockly     TEXT    NOT NULL,
    tree        TEXT    NOT NULL,
    problems    TEXT    NOT NULL,
    PRIMARY KEY (prompt_id, program_id)
);
