CREATE TABLE IF NOT EXISTS r5_scan_state (
    source VARCHAR(64) NOT NULL PRIMARY KEY,
    after_id BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS r5_deliveries (
    message_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    payload_hash CHAR(64) NOT NULL,
    envelope JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    published_at DATETIME(3) NULL,
    last_error VARCHAR(64) NULL,
    KEY idx_r5_delivery_due(status,next_attempt_at)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS r5_receipts (
    message_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    payload_hash CHAR(64) NOT NULL,
    fingerprint CHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error VARCHAR(64) NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS r5_report_runs (
    source VARCHAR(64) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    payload_hash CHAR(64) NOT NULL,
    report JSON NOT NULL,
    PRIMARY KEY(source,run_id)
) ENGINE=InnoDB;
