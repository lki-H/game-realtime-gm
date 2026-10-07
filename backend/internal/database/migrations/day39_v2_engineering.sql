CREATE TABLE IF NOT EXISTS pve_run_archives (
    run_id VARCHAR(64) NOT NULL,
    final_sequence BIGINT NOT NULL,
    summary JSON NOT NULL,
    fingerprint CHAR(64) NOT NULL,
    event_count BIGINT NOT NULL,
    application_count BIGINT NOT NULL,
    archived_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
