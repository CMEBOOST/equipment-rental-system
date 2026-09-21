-- Restoring the unconditional UNIQUE constraints can fail if a soft-deleted
-- row now shares an email/username with a live one, which is exactly the
-- situation the up migration allows. That is inherent to reverting this
-- change; clean up the duplicates first if it happens.
DROP INDEX IF EXISTS idx_users_email_active;
DROP INDEX IF EXISTS idx_users_username_active;

ALTER TABLE users ADD CONSTRAINT users_email_key    UNIQUE (email);
ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE (username);
