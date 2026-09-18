CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE roles (
    id          SMALLINT     PRIMARY KEY,
    name        VARCHAR(20)  NOT NULL UNIQUE,
    description VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE TABLE users (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    username      VARCHAR(50)  NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(120) NOT NULL DEFAULT '',
    phone         VARCHAR(20)  NOT NULL DEFAULT '',
    role_id       SMALLINT     NOT NULL REFERENCES roles(id),
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ  NULL
);
CREATE INDEX idx_users_role_id    ON users(role_id);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

CREATE TABLE refresh_tokens (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(64)  NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ  NOT NULL,
    revoked_at  TIMESTAMPTZ  NULL,
    ip_address  VARCHAR(45)  NOT NULL DEFAULT '',
    user_agent  VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

CREATE TABLE login_logs (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         UUID         NULL REFERENCES users(id) ON DELETE SET NULL,
    email_attempted VARCHAR(255) NOT NULL,
    success         BOOLEAN      NOT NULL,
    ip_address      VARCHAR(45)  NOT NULL DEFAULT '',
    user_agent      VARCHAR(255) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_login_logs_user_id    ON login_logs(user_id);
CREATE INDEX idx_login_logs_created_at ON login_logs(created_at);
