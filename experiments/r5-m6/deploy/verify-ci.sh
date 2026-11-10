#!/usr/bin/env bash
set -euo pipefail

module_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
project_root="$(cd "$module_root/../.." && pwd)"
compose_file="$module_root/deploy/compose.yaml"
evidence="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/r5-ci.XXXXXX")"
chmod 700 "$evidence"
mkdir "$evidence/bin"
role_pids=()
cleanup() {
  for role_pid in "${role_pids[@]}"; do kill "$role_pid" 2>/dev/null || true; wait "$role_pid" 2>/dev/null || true; done
  docker compose -f "$compose_file" --profile main down -v
  rm -f "$evidence/admin.json" "$evidence/runtime.json" "$evidence/clients.json" "$evidence/source.dump" "$evidence/rpc.json" "$evidence/bridge.json" "$evidence/publisher.json" "$evidence/consumer.json" "$evidence/ops.json" "$evidence/query.json"
}
trap cleanup EXIT

for variable in PVE_TEST_PASSWORD R5_ADMIN_PASSWORD R5_MAIN_JWT R5_MAIN_EVENT_TOKEN R5_MAIN_PLAYER_PASSWORD; do
  printf -v "$variable" '%s' "$(openssl rand -hex 32)"
  export "$variable"
  if [[ "${GITHUB_ACTIONS:-}" == true ]]; then printf '::add-mask::%s\n' "${!variable}"; fi
done
export R5_BINARY_DIRECTORY="$evidence/bin"
export DB_HOST=127.0.0.1 DB_PORT=23306 DB_NAME=game_realtime_v2_test DB_USER=pve_test
export DB_PASSWORD="$PVE_TEST_PASSWORD" JWT_SECRET="$R5_MAIN_JWT" GAMEPLAY_MODE=v2 GIN_MODE=release APP_HOST=127.0.0.1
export R5_ACCEPTANCE_FAULT=""
export V2_MIGRATION_CONFIRM=I_UNDERSTAND_V2_MIGRATION V2_BACKUP_CONFIRM=I_UNDERSTAND_V2_BACKUP

if [[ -n "$(docker ps -a --filter label=com.docker.compose.project=gm-r5-verify --format '{{.ID}}')" ]]; then
  printf '%s\n' 'An isolated R5 environment already exists; refusing to reuse it.' >&2
  trap - EXIT
  exit 1
fi

cd "$project_root/backend"
go build -o "$evidence/bin/main-linux" ./cmd/server
go build -o "$evidence/bin/verify-linux" ./cmd/tools/pve_verify
go build -o "$evidence/bin/migrate" ./cmd/tools/v2_migrate
go build -o "$evidence/bin/backup" ./cmd/tools/v2_backup
docker compose -f "$compose_file" up -d --wait --wait-timeout 180 mysql redis rabbitmq
"$evidence/bin/migrate" -stage r4
docker compose -f "$compose_file" --profile main up -d main
ready=false
for attempt in $(seq 1 60); do
  if curl --fail --silent http://127.0.0.1:8080/health >/dev/null; then ready=true; break; fi
  sleep 1
done
if [[ "$ready" != true ]]; then printf '%s\n' 'Primary fixture did not become ready.' >&2; exit 1; fi
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode prepare
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode scenario
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode check
"$evidence/bin/backup" -action backup -container gm-r5-verify-mysql-1 -database game_realtime_v2_test -file "$evidence/source.dump"
"$evidence/bin/backup" -action restore -container gm-r5-verify-mysql-1 -database game_realtime_v2_r5_restore -file "$evidence/source.dump"

export R5_CI_EVIDENCE="$evidence"
python3 - <<'PY'
import json
import os
from pathlib import Path

configuration = {
    "root_db": {"address": "127.0.0.1:23306", "name": "game_realtime_v2_r5_restore", "user": "root", "password": os.environ["PVE_TEST_PASSWORD"]},
    "broker": {"address": "127.0.0.1:25672", "vhost": "r5", "user": "r5_admin", "password": os.environ["R5_ADMIN_PASSWORD"]},
    "management_address": "127.0.0.1:25673",
}
path = Path(os.environ["R5_CI_EVIDENCE"]) / "admin.json"
path.write_text(json.dumps(configuration), encoding="utf-8")
path.chmod(0o600)
PY

cd "$module_root"
go run ./cmd/prepare -admin "$evidence/admin.json" -output "$evidence/runtime.json"
go build -o "$evidence/bin/r5-linux" ./cmd/r5
go build -o "$evidence/bin/query-linux" ./cmd/query
go build -o "$evidence/bin/r5ops-linux" ./cmd/r5ops
export R5_INTEGRATION=1 R5_CONFIG="$evidence/runtime.json" R5_ADMIN_CONFIG="$evidence/admin.json"
export R5_PROCESS_BINARY="$evidence/bin/r5-linux"
go test -json -race -count=1 -timeout 240s ./... | tee "$evidence/r5-integration-events.jsonl"
python3 "$project_root/deploy/review_checks.py" --test-events "$evidence/r5-integration-events.jsonl" --kind r5-integration --required TestIntegrationConsumerProcessAfterCommitRecovery --required TestIntegrationManualRetryKeepsBudgetAndReportsOnly --required TestSharedGoCSharpGoldenBytes --required TestRoleConfigurationContainsOnlyRequiredDependencies
go vet ./...
go mod verify

report_sql() {
  printf '%s\n' "$1" | docker exec -i gm-r5-verify-mysql-1 sh -lc 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -N -B -uroot game_realtime_r5_reports'
}
queue_ready() {
  docker exec gm-r5-verify-rabbitmq-1 rabbitmqctl -q list_queues -p r5 name messages_ready | awk '$1=="r5.reports" {print $2}'
}
start_role() {
  "$evidence/bin/r5-linux" -mode "$1" -config "$evidence/$1.json" >"$evidence/$1.stdout.log" 2>"$evidence/$1.stderr.log" &
  role_pids+=("$!")
}
stop_roles() {
  for role_pid in "${role_pids[@]}"; do kill "$role_pid" 2>/dev/null || true; wait "$role_pid" 2>/dev/null || true; done
  role_pids=()
}
report_sql 'DELETE FROM r5_report_runs; DELETE FROM r5_receipts; DELETE FROM r5_deliveries; DELETE FROM r5_scan_state;'
start_role bridge
start_role publisher
published=false
for attempt in $(seq 1 60); do
  if [[ "$(report_sql "SELECT COUNT(*) FROM r5_deliveries WHERE status='published'")" -ge 1 && "$(report_sql "SELECT COUNT(*) FROM r5_deliveries WHERE status<>'published'")" -eq 0 ]]; then published=true; break; fi
  sleep 1
done
if [[ "$published" != true ]]; then printf '%s\n' 'Confirmed backlog was not prepared.' >&2; exit 1; fi
stop_roles
backlog_before="$(queue_ready)"
if [[ "$backlog_before" -lt 1 ]]; then printf '%s\n' 'Confirmed backlog is missing.' >&2; exit 1; fi
docker compose -f "$compose_file" stop rabbitmq
# RPC receives only its source reader and service identities, even while MQ is down.
start_role rpc
rpc_ready=false
for attempt in $(seq 1 30); do
  if "$evidence/bin/query-linux" -config "$evidence/query.json" -clients "$evidence/clients.json" >"$evidence/rpc-independent.json" 2>/dev/null; then rpc_ready=true; break; fi
  sleep 1
done
if [[ "$rpc_ready" != true ]]; then printf '%s\n' 'RPC failed without broker/projection credentials.' >&2; exit 1; fi
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode scenario
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode check
stop_roles
docker compose -f "$compose_file" up -d --wait --wait-timeout 120 rabbitmq
if [[ "$(queue_ready)" -lt "$backlog_before" ]]; then printf '%s\n' 'Broker restart lost confirmed backlog.' >&2; exit 1; fi
start_role consumer
recovered=false
for attempt in $(seq 1 60); do
  expected="$(report_sql 'SELECT COUNT(*) FROM r5_deliveries')"
  applied="$(report_sql "SELECT COUNT(*) FROM r5_receipts WHERE status='applied'")"
  if [[ "$applied" -eq "$expected" && "$(queue_ready)" -eq 0 ]]; then recovered=true; break; fi
  sleep 1
done
if [[ "$recovered" != true ]]; then printf '%s\n' 'Persisted backlog did not recover.' >&2; exit 1; fi
conflicts="$(report_sql "SELECT COUNT(*) FROM r5_deliveries d LEFT JOIN r5_report_runs r ON r.source=JSON_UNQUOTE(JSON_EXTRACT(d.envelope,'$.source')) AND r.run_id=JSON_UNQUOTE(JSON_EXTRACT(d.envelope,'$.payload.run_id')) WHERE r.payload_hash IS NULL OR r.payload_hash<>d.payload_hash")"
if [[ "$conflicts" -ne 0 ]]; then printf '%s\n' 'Recovered report content differs from the original delivery.' >&2; exit 1; fi
"$evidence/bin/r5ops-linux" -config "$evidence/ops.json" >"$evidence/inspection.json"
stop_roles
printf '%s\n' 'R5 real storage/race, actual consumer crash, durable broker restart, independent RPC and primary settlement isolation: PASS'
