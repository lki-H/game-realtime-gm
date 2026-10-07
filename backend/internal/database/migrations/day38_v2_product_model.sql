CREATE TABLE IF NOT EXISTS pve_regroup_proposals (
    id VARCHAR(64) NOT NULL,
    source_run_id VARCHAR(64) NOT NULL,
    proposer_id BIGINT NOT NULL,
    owner_id BIGINT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    plan JSON NOT NULL,
    sources JSON NOT NULL,
    result_party_id VARCHAR(64) NULL,
    expires_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pve_regroup_expiry (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_regroup_members (
    proposal_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    response VARCHAR(32) NOT NULL DEFAULT 'pending',
    PRIMARY KEY (proposal_id, player_id),
    KEY idx_pve_regroup_player (player_id, proposal_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_recruitment_preferences (
    party_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    ready TINYINT(1) NOT NULL DEFAULT 0,
    selection_version BIGINT NOT NULL DEFAULT 1,
    task_selection JSON NOT NULL,
    roster_version BIGINT NOT NULL,
    plan_version BIGINT NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    PRIMARY KEY (party_id, player_id),
    UNIQUE KEY uq_pve_recruitment_player (player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_match_ticket_plans (
    ticket_id VARCHAR(64) NOT NULL,
    rule_version VARCHAR(64) NOT NULL,
    cohort_id VARCHAR(64) NULL,
    source_kind VARCHAR(32) NOT NULL,
    PRIMARY KEY (ticket_id),
    KEY idx_pve_ticket_cohort (cohort_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_task_periods (
    player_id BIGINT NOT NULL,
    task_key VARCHAR(64) NOT NULL,
    task_version VARCHAR(64) NOT NULL,
    period_id VARCHAR(64) NOT NULL,
    progress JSON NOT NULL,
    completed TINYINT(1) NOT NULL DEFAULT 0,
    PRIMARY KEY (player_id, task_key, task_version, period_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_task_completions (
    player_id BIGINT NOT NULL,
    task_key VARCHAR(64) NOT NULL,
    task_version VARCHAR(64) NOT NULL,
    period_id VARCHAR(64) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    attempt_id VARCHAR(64) NOT NULL,
    confirmed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_id, task_key, task_version, period_id),
    UNIQUE KEY uq_pve_task_completion_attempt (attempt_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_task_pauses (
    player_id BIGINT NOT NULL,
    task_key VARCHAR(64) NOT NULL,
    task_version VARCHAR(64) NOT NULL,
    paused TINYINT(1) NOT NULL DEFAULT 1,
    PRIMARY KEY (player_id, task_key, task_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_contribution_evidence (
    event_id VARCHAR(128) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    contribution_type VARCHAR(32) NOT NULL,
    target_id VARCHAR(128) NOT NULL,
    PRIMARY KEY (event_id, player_id),
    KEY idx_pve_contribution_run (run_id, player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
