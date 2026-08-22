CREATE TABLE IF NOT EXISTS players (
    id BIGINT NOT NULL AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'normal',
    banned_reason TEXT NOT NULL,
    banned_at DATETIME(3) NULL,
    banned_by_admin_id BIGINT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_players_username (username),
    KEY idx_players_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS admins (
    id BIGINT NOT NULL AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'gm',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_admins_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS admin_operation_logs (
    id BIGINT NOT NULL AUTO_INCREMENT,
    admin_id BIGINT NOT NULL,
    admin_username VARCHAR(64) NOT NULL,
    admin_role VARCHAR(32) NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(64) NOT NULL,
    target_id BIGINT NULL,
    detail TEXT NOT NULL,
    ip VARCHAR(64) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_admin_operation_logs_admin_id (admin_id),
    KEY idx_admin_operation_logs_action (action),
    KEY idx_admin_operation_logs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;


CREATE TABLE IF NOT EXISTS mission_records (
    id BIGINT NOT NULL AUTO_INCREMENT,
    mission_instance_id VARCHAR(64) NOT NULL,
    mission_id VARCHAR(64) NOT NULL,
    squad_id VARCHAR(64) NOT NULL,
    submitted_by_player_id BIGINT NOT NULL,
    nonce VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'settled',
    completion_seconds BIGINT NOT NULL,
    score BIGINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_mission_records_instance_id (mission_instance_id),
    UNIQUE KEY uq_mission_records_idempotency_key (idempotency_key),
    UNIQUE KEY uq_mission_records_player_nonce (submitted_by_player_id, nonce),
    KEY idx_mission_records_player_id (submitted_by_player_id),
    KEY idx_mission_records_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS reward_records (
    id BIGINT NOT NULL AUTO_INCREMENT,
    mission_record_id BIGINT NOT NULL,
    mission_instance_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    reward_type VARCHAR(64) NOT NULL,
    amount BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'granted',
    granted_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_reward_records_mission_player_type (mission_record_id, player_id, reward_type),
    KEY idx_reward_records_mission_record_id (mission_record_id),
    KEY idx_reward_records_instance_id (mission_instance_id),
    KEY idx_reward_records_player_id (player_id),
    KEY idx_reward_records_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS asset_ledger (
    id BIGINT NOT NULL AUTO_INCREMENT,
    player_id BIGINT NOT NULL,
    mission_record_id BIGINT NOT NULL,
    reward_record_id BIGINT NOT NULL,
    mission_instance_id VARCHAR(64) NOT NULL,
    asset_type VARCHAR(64) NOT NULL,
    delta BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    reason VARCHAR(64) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_asset_ledger_reward_record_id (reward_record_id),
    KEY idx_asset_ledger_player_id (player_id),
    KEY idx_asset_ledger_mission_record_id (mission_record_id),
    KEY idx_asset_ledger_instance_id (mission_instance_id),
    KEY idx_asset_ledger_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS player_assets (
    player_id BIGINT NOT NULL,
    soft_currency BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
