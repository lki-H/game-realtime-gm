ALTER TABLE mission_records
    ADD COLUMN idempotency_key VARCHAR(64) NULL AFTER nonce;

UPDATE mission_records
SET idempotency_key = CONCAT('legacy_mission_record_', id)
WHERE idempotency_key IS NULL;

ALTER TABLE mission_records
    MODIFY COLUMN idempotency_key VARCHAR(64) NOT NULL,
    MODIFY COLUMN status VARCHAR(32) NOT NULL DEFAULT 'settled',
    DROP INDEX idx_mission_records_instance_id,
    ADD UNIQUE KEY uq_mission_records_instance_id (mission_instance_id),
    ADD UNIQUE KEY uq_mission_records_idempotency_key (idempotency_key);

ALTER TABLE reward_records
    MODIFY COLUMN status VARCHAR(32) NOT NULL DEFAULT 'granted',
    ADD COLUMN granted_at DATETIME(3) NULL AFTER status,
    ADD COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER created_at,
    ADD UNIQUE KEY uq_reward_records_mission_player_type (mission_record_id, player_id, reward_type);

CREATE TABLE IF NOT EXISTS player_assets (
    player_id BIGINT NOT NULL,
    soft_currency BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_id)
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

INSERT INTO player_assets (player_id, soft_currency, created_at, updated_at)
SELECT id, 0, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
FROM players
ON DUPLICATE KEY UPDATE updated_at = player_assets.updated_at;
