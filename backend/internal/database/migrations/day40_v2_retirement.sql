CREATE TABLE IF NOT EXISTS pve_service_control (
    id VARCHAR(32) NOT NULL,
    admission_state VARCHAR(32) NOT NULL DEFAULT 'open',
    version BIGINT NOT NULL DEFAULT 1,
    reason VARCHAR(512) NOT NULL DEFAULT '',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_control_operations (
    operation_id VARCHAR(128) NOT NULL,
    admin_id BIGINT NOT NULL,
    fingerprint CHAR(64) NOT NULL,
    result JSON NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (operation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
