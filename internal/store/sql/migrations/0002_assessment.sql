-- Migration to schema version 2 (config.md section 4.5.4): adds the
-- assessment column to runs, nullable, with a CHECK naming the same four
-- values as review.schema.json's assessment enum, mirroring verdict's own
-- CHECK exactly (config.md section 4.5.2). Existing rows get NULL for it,
-- and NULL IN (...) evaluates to NULL rather than false, so the CHECK
-- accepts every row this ALTER TABLE touches without a special case.
ALTER TABLE runs ADD COLUMN assessment TEXT CHECK (assessment IN ('strong', 'sound', 'mixed', 'weak'));
