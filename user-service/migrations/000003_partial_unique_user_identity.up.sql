-- users.email and users.username were declared UNIQUE at the column level in
-- 000001, which makes the constraint unconditional: it keeps matching rows
-- that have been soft-deleted (deleted_at IS NOT NULL). Every read path in
-- the service, however, goes through GORM's DeletedAt scoping and therefore
-- cannot see those rows. The result was that deleting a user permanently
-- burned their email and username: the duplicate pre-check found nothing,
-- the INSERT then failed on a constraint nobody could explain, and the
-- caller got a bare 409 CONFLICT.
--
-- Scoping the uniqueness to live rows makes the database agree with what the
-- application can actually see, so a deleted account's email/username become
-- available again -- which is what users expect after their account is
-- removed -- while two live users still cannot share either value.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_key;

CREATE UNIQUE INDEX idx_users_email_active    ON users (email)    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_username_active ON users (username) WHERE deleted_at IS NULL;
