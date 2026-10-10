#!/usr/bin/env bash
set -euo pipefail

module_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
project_root="$(cd "$module_root/../.." && pwd)"
compose_file="$module_root/deploy/compose.yaml"
evidence="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/r5-ci.XXXXXX")"
chmod 700 "$evidence"
mkdir "$evidence/bin"
cleanup() {
  docker compose -f "$compose_file" --profile main down -v
  rm -f "$evidence/admin.json" "$evidence/runtime.json" "$evidence/clients.json" "$evidence/source.dump"
}
trap cleanup EXIT

for variable in PVE_TEST_PASSWORD R5_ADMIN_PASSWORD R5_MAIN_JWT R5_MAIN_EVENT_TOKEN R5_MAIN_PLAYER_PASSWORD; do
  printf -v "$variable" '%s' "$(openssl rand -hex 32)"
  export "$variable"
  if [[ "${GITHUB_ACTIONS:-}" == true ]]; then printf '::add-mask::%s\n' "${!variable}"; fi
done
export R5_BINARY_DIRECTORY="$evidence/bin"
export DB_HOST=127.0.0.1 DB_PORT=23306 DB_NAME=game_realtime_v2_test DB_USER=pve_test
export DB_PASSWORD="$PVE_TEST_PASSWORD" GAMEPLAY_MODE=v2 GIN_MODE=release
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
export R5_INTEGRATION=1 R5_CONFIG="$evidence/runtime.json" R5_ADMIN_CONFIG="$evidence/admin.json"
go test -race -count=1 -timeout 180s ./...
go vet ./...
go mod verify
docker compose -f "$compose_file" stop rabbitmq
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode scenario
docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode check
printf '%s\n' 'R5 real MySQL/RabbitMQ integration, race and primary settlement isolation: PASS'
