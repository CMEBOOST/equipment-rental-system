-- Seeds the first admin and staff accounts. Without this, every account
-- created through POST /auth/register gets role_id 3 (customer) -- there
-- was no way to reach the admin-only endpoints (POST /users,
-- PATCH /users/:id/role) that assign any other role, because reaching them
-- already requires being an admin. This breaks that chicken-and-egg problem
-- for a fresh database.
--
-- Passwords are dev-only placeholders (bcrypt cost 12, matching
-- BCRYPT_COST in .env.example) -- change them immediately in any real
-- deployment. See user-service/README.md for the plaintext values.
INSERT INTO users (email, username, password_hash, full_name, role_id, is_active)
VALUES
    ('admin@equipment-rental.local', 'admin',
     '$2a$12$KlrvYfyxkaHG9/d89Ya15eiDfH3CN.ZE.SaDtoHw.vNm0MlOOqruC',
     'System Admin', 1, TRUE),
    ('staff@equipment-rental.local', 'staff',
     '$2a$12$Y6fkAP/NrTlceLthFc8IBesI1elMWvFLMIHo1KsqbcCc32HqledD.',
     'System Staff', 2, TRUE)
-- 000003 replaced the plain UNIQUE(email) constraint with a partial unique
-- index scoped to live rows, so ON CONFLICT inference must match that same
-- predicate.
ON CONFLICT (email) WHERE deleted_at IS NULL DO NOTHING;
