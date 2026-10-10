CREATE OR REPLACE SQL SECURITY DEFINER VIEW r5_runs AS
SELECT id,operation_name,difficulty,end_reason FROM pve_runs
WHERE status='closed' AND JSON_EXTRACT(state,'$.settled')=true;
CREATE OR REPLACE SQL SECURITY DEFINER VIEW r5_results AS
SELECT p.run_id,p.player_id,p.contribution_qualified,p.task_completed,p.settled_at,
COALESCE((SELECT SUM(g.amount) FROM pve_reward_grants g WHERE g.run_id=p.run_id AND g.player_id=p.player_id AND g.status='granted'),0) AS reward
FROM pve_participant_results p JOIN r5_runs r ON r.id=p.run_id
WHERE p.result_status='settled';
CREATE OR REPLACE SQL SECURITY DEFINER VIEW r5_outbox AS
SELECT o.id,o.created_at,r.id AS run_id FROM pve_outbox_records o
JOIN r5_runs r ON r.id=JSON_UNQUOTE(JSON_EXTRACT(o.payload,'$.data.run_id'))
WHERE o.event_type='v2.run.result';
CREATE OR REPLACE SQL SECURITY DEFINER VIEW r5_leaderboard AS
SELECT player_id,SUM(reward) AS score FROM r5_results GROUP BY player_id;
