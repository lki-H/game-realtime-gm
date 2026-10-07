CREATE TABLE IF NOT EXISTS pve_social_friend_requests (
    id BIGINT NOT NULL AUTO_INCREMENT,
    requester_id BIGINT NOT NULL,
    recipient_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_friend_request_pair (requester_id, recipient_id),
    KEY idx_pve_friend_request_recipient (recipient_id, status),
    KEY idx_pve_friend_request_requester (requester_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_social_friendships (
    player_low_id BIGINT NOT NULL,
    player_high_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_low_id, player_high_id),
    KEY idx_pve_friendship_high (player_high_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_social_blocks (
    blocker_id BIGINT NOT NULL,
    blocked_id BIGINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (blocker_id, blocked_id),
    KEY idx_pve_social_blocks_blocked (blocked_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_social_notes (
    owner_id BIGINT NOT NULL,
    target_id BIGINT NOT NULL,
    note VARCHAR(128) NOT NULL DEFAULT '',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (owner_id, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_social_messages (
    id BIGINT NOT NULL AUTO_INCREMENT,
    sender_id BIGINT NOT NULL,
    recipient_id BIGINT NOT NULL,
    body VARCHAR(512) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pve_message_conversation (sender_id, recipient_id, id),
    KEY idx_pve_message_recipient (recipient_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_social_message_reads (
    player_id BIGINT NOT NULL,
    peer_id BIGINT NOT NULL,
    last_read_message_id BIGINT NOT NULL DEFAULT 0,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_id, peer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_parties (
    id VARCHAR(64) NOT NULL,
    owner_id BIGINT NOT NULL,
    join_policy VARCHAR(32) NOT NULL DEFAULT 'invite_only',
    fill_policy VARCHAR(32) NOT NULL DEFAULT 'no_fill',
    status VARCHAR(32) NOT NULL DEFAULT 'open',
    roster_version BIGINT NOT NULL DEFAULT 1,
    plan_version BIGINT NOT NULL DEFAULT 1,
    operation_id VARCHAR(64) NULL,
    plan JSON NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pve_party_owner (owner_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_party_members (
    membership_id VARCHAR(64) NOT NULL,
    party_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    ready TINYINT(1) NOT NULL DEFAULT 0,
    task_selection JSON NULL,
    selection_version BIGINT NOT NULL DEFAULT 1,
    active_player_id BIGINT GENERATED ALWAYS AS (CASE WHEN status = 'active' THEN player_id ELSE NULL END) STORED,
    joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (membership_id),
    UNIQUE KEY uq_pve_party_player_active (active_player_id),
    KEY idx_pve_party_member_party (party_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_party_invites (
    id BIGINT NOT NULL AUTO_INCREMENT,
    party_id VARCHAR(64) NOT NULL,
    inviter_id BIGINT NOT NULL,
    invitee_id BIGINT NOT NULL,
    token_hash CHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    expires_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_party_invite_token (token_hash),
    KEY idx_pve_party_invite_recipient (invitee_id, status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_recruitment_posts (
    id VARCHAR(64) NOT NULL,
    party_id VARCHAR(64) NOT NULL,
    owner_id BIGINT NOT NULL,
    operation_id VARCHAR(64) NOT NULL,
    difficulty VARCHAR(32) NOT NULL,
    slots INT NOT NULL,
    tags JSON NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'open',
    expires_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pve_recruitment_status (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_recruitment_applications (
    id BIGINT NOT NULL AUTO_INCREMENT,
    post_id VARCHAR(64) NOT NULL,
    applicant_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_recruitment_applicant (post_id, applicant_id),
    KEY idx_pve_recruitment_application_status (post_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_match_tickets (
    id VARCHAR(64) NOT NULL,
    source_party_id VARCHAR(64) NULL,
    operation_id VARCHAR(64) NOT NULL,
    operation_name VARCHAR(64) NOT NULL,
    difficulty VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    fill_policy VARCHAR(32) NOT NULL DEFAULT 'no_fill',
    queue_priority_since DATETIME(3) NOT NULL,
    allow_partial TINYINT(1) NOT NULL DEFAULT 1,
    stage_entered_at DATETIME(3) NOT NULL,
    deadline_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_match_ticket_operation (operation_id),
    KEY idx_pve_match_ticket_queue (status, operation_name, difficulty, queue_priority_since)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_match_ticket_members (
    ticket_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    membership_id VARCHAR(64) NULL,
    source_kind VARCHAR(32) NOT NULL DEFAULT 'solo',
    selection_version BIGINT NOT NULL DEFAULT 1,
    task_selection JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (ticket_id, player_id),
    KEY idx_pve_match_ticket_member_player (player_id, ticket_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_match_proposals (
    id VARCHAR(64) NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    operation_name VARCHAR(64) NOT NULL,
    difficulty VARCHAR(32) NOT NULL,
    run_id VARCHAR(64) NULL,
    deadline_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pve_match_proposal_status (status, deadline_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_match_proposal_members (
    proposal_id VARCHAR(64) NOT NULL,
    ticket_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    response VARCHAR(32) NOT NULL DEFAULT 'pending',
    responded_at DATETIME(3) NULL,
    PRIMARY KEY (proposal_id, player_id),
    KEY idx_pve_match_proposal_ticket (proposal_id, ticket_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_player_activity_locks (
    player_id BIGINT NOT NULL,
    activity_type VARCHAR(32) NOT NULL,
    activity_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_id),
    KEY idx_pve_activity_lock_activity (activity_type, activity_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_rule_versions (
    rule_key VARCHAR(128) NOT NULL,
    version VARCHAR(64) NOT NULL,
    content_hash CHAR(64) NOT NULL,
    content JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'published',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (rule_key, version),
    UNIQUE KEY uq_pve_rule_hash (rule_key, content_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_runs (
    id VARCHAR(64) NOT NULL,
    operation_name VARCHAR(64) NOT NULL,
    difficulty VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'provisioning',
    end_reason VARCHAR(32) NULL,
    rule_version VARCHAR(64) NOT NULL,
    state JSON NOT NULL,
    source_generation BIGINT NOT NULL DEFAULT 1,
    reinforcement_budget INT NOT NULL,
    reinforcement_reserved INT NOT NULL DEFAULT 0,
    reinforcement_used INT NOT NULL DEFAULT 0,
    started_at DATETIME(3) NULL,
    deadline_at DATETIME(3) NULL,
    final_sequence BIGINT NULL,
    ended_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pve_run_status (status, deadline_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_run_participants (
    participant_id VARCHAR(64) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    source_ticket_id VARCHAR(64) NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'assigned',
    life_status VARCHAR(32) NOT NULL DEFAULT 'alive',
    connection_generation BIGINT NOT NULL DEFAULT 1,
    task_attempt_id VARCHAR(64) NULL,
    left_sequence BIGINT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (participant_id),
    UNIQUE KEY uq_pve_run_player (run_id, player_id),
    KEY idx_pve_participant_player (player_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_run_events (
    event_id VARCHAR(128) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    source VARCHAR(64) NOT NULL,
    source_generation BIGINT NOT NULL,
    sequence_no BIGINT NOT NULL,
    event_type VARCHAR(32) NOT NULL,
    actor_player_id BIGINT NULL,
    contributors JSON NULL,
    target_id VARCHAR(128) NULL,
    payload JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'received',
    fingerprint CHAR(64) NOT NULL,
    occurred_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id),
    UNIQUE KEY uq_pve_event_sequence (run_id, source, source_generation, sequence_no),
    KEY idx_pve_event_run_sequence (run_id, source_generation, sequence_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_event_applications (
    event_id VARCHAR(128) NOT NULL,
    player_id BIGINT NOT NULL,
    objective_key VARCHAR(128) NOT NULL,
    progress_delta BIGINT NOT NULL DEFAULT 0,
    applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id, player_id, objective_key),
    KEY idx_pve_event_application_player (player_id, objective_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_run_objectives (
    run_id VARCHAR(64) NOT NULL,
    objective_key VARCHAR(128) NOT NULL,
    objective_type VARCHAR(32) NOT NULL,
    required_count BIGINT NOT NULL DEFAULT 1,
    shared TINYINT(1) NOT NULL DEFAULT 1,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    progress BIGINT NOT NULL DEFAULT 0,
    completed_at DATETIME(3) NULL,
    PRIMARY KEY (run_id, objective_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_player_task_attempts (
    id VARCHAR(64) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    task_key VARCHAR(128) NULL,
    task_version VARCHAR(64) NULL,
    compatibility_status VARCHAR(32) NOT NULL DEFAULT 'compatible',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_task_attempt_player (run_id, player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_player_task_progress (
    attempt_id VARCHAR(64) NOT NULL,
    objective_key VARCHAR(128) NOT NULL,
    objective_type VARCHAR(32) NOT NULL,
    contribution_scope VARCHAR(32) NOT NULL DEFAULT 'self',
    required_count BIGINT NOT NULL DEFAULT 1,
    progress BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    completed_at DATETIME(3) NULL,
    PRIMARY KEY (attempt_id, objective_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_reinforcement_actions (
    action_id VARCHAR(128) NOT NULL,
    run_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_spawn',
    deadline_at DATETIME(3) NOT NULL,
    reserved_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    resolved_at DATETIME(3) NULL,
    PRIMARY KEY (action_id),
    KEY idx_pve_reinforcement_run (run_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_participant_results (
    run_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    result_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    contribution_qualified TINYINT(1) NOT NULL DEFAULT 0,
    task_completed TINYINT(1) NOT NULL DEFAULT 0,
    settled_at DATETIME(3) NULL,
    PRIMARY KEY (run_id, player_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_reward_grants (
    id BIGINT NOT NULL AUTO_INCREMENT,
    run_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    reward_source VARCHAR(64) NOT NULL,
    asset_type VARCHAR(64) NOT NULL,
    amount BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_reward_source (run_id, player_id, reward_source),
    KEY idx_pve_reward_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_pending_operations (
    operation_id VARCHAR(128) NOT NULL,
    operation_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(128) NOT NULL,
    payload JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(3) NOT NULL,
    last_error TEXT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (operation_id),
    KEY idx_pve_pending_due (status, next_attempt_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_outbox_records (
    id BIGINT NOT NULL AUTO_INCREMENT,
    operation_id VARCHAR(128) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    aggregate_id VARCHAR(128) NOT NULL,
    payload JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    published_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_pve_outbox_operation (operation_id),
    KEY idx_pve_outbox_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pve_command_results (
    player_id BIGINT NOT NULL,
    operation_id VARCHAR(128) NOT NULL,
    action VARCHAR(64) NOT NULL,
    fingerprint CHAR(64) NOT NULL,
    result JSON NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (player_id, operation_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS pve_player_tasks (
    player_id BIGINT NOT NULL,
    task_key VARCHAR(128) NOT NULL,
    version VARCHAR(64) NOT NULL,
    progress JSON NOT NULL,
    completed TINYINT(1) NOT NULL DEFAULT 0,
    unlocked TINYINT(1) NOT NULL DEFAULT 1,
    PRIMARY KEY (player_id, task_key, version)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS pve_recruitment_roster (
    party_id VARCHAR(64) NOT NULL,
    player_id BIGINT NOT NULL,
    application_id BIGINT NOT NULL,
    PRIMARY KEY (party_id, player_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS pve_match_lanes (id VARCHAR(128) NOT NULL PRIMARY KEY) ENGINE=InnoDB;
INSERT IGNORE INTO pve_match_lanes (id) VALUES ('training_ground:normal');

CREATE TABLE IF NOT EXISTS pve_session_revocations (
    player_id BIGINT NOT NULL PRIMARY KEY,
    revoked_through DATETIME(3) NOT NULL
) ENGINE=InnoDB;

ALTER TABLE asset_ledger
    MODIFY mission_record_id BIGINT NULL,
    MODIFY reward_record_id BIGINT NULL,
    MODIFY mission_instance_id VARCHAR(64) NULL,
    ADD pve_grant_id BIGINT NULL,
    ADD pve_run_id VARCHAR(64) NULL,
    ADD UNIQUE KEY uq_asset_ledger_pve_grant (pve_grant_id);

