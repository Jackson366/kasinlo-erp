CREATE TABLE IF NOT EXISTS device_authorizations (
    device_hash BYTEA PRIMARY KEY,
    user_code TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    next_poll_at TIMESTAMPTZ NOT NULL,
    poll_interval INTEGER NOT NULL DEFAULT 5,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'denied', 'consumed')),
    user_id BIGINT,
    token_epoch BIGINT
);

CREATE TABLE IF NOT EXISTS access_grants (
    id TEXT PRIMARY KEY,
    token_hash BYTEA NOT NULL UNIQUE,
    client_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    token_epoch BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS device_authorizations_expires_idx
    ON device_authorizations (expires_at);
CREATE INDEX IF NOT EXISTS access_grants_user_idx
    ON access_grants (user_id, expires_at);
CREATE INDEX IF NOT EXISTS access_grants_expires_idx
    ON access_grants (expires_at);
