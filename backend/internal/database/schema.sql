CREATE TABLE IF NOT EXISTS players (
                                       id BIGSERIAL PRIMARY KEY,
                                       username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'normal',
    banned_reason TEXT NOT NULL DEFAULT '',
    banned_at TIMESTAMPTZ,
    banned_by_admin_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );

CREATE INDEX IF NOT EXISTS idx_players_status
    ON players (status);

CREATE TABLE IF NOT EXISTS admins (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'gm',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS admin_operation_logs (
                                                    id BIGSERIAL PRIMARY KEY,
                                                    admin_id BIGINT NOT NULL,
                                                    admin_username VARCHAR(64) NOT NULL,
    admin_role VARCHAR(32) NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(64) NOT NULL,
    target_id BIGINT,
    detail TEXT NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    );

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_admin_id
    ON admin_operation_logs (admin_id);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_action
    ON admin_operation_logs (action);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_created_at
    ON admin_operation_logs (created_at DESC);