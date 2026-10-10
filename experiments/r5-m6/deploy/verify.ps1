param([switch]$KeepEnvironment, [string]$EvidenceRoot = (Join-Path ([IO.Path]::GetTempPath()) 'pve-r5-acceptance'))
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$moduleRoot = Split-Path -Parent $PSScriptRoot
$projectRoot = Split-Path -Parent (Split-Path -Parent $moduleRoot)
$backendRoot = Join-Path $projectRoot 'backend'
$compose = Join-Path $PSScriptRoot 'compose.yaml'
$projectName = 'gm-r5-verify'
$roles = @{}
$composeStarted = $false
$evidence = Join-Path $EvidenceRoot ([guid]::NewGuid().ToString('N'))
$variables = @('PVE_TEST_PASSWORD','PVE_INTEGRATION','PVE_TEST_NETWORK','R5_ADMIN_PASSWORD','R5_MAIN_JWT','R5_MAIN_EVENT_TOKEN','R5_MAIN_PLAYER_PASSWORD','R5_BINARY_DIRECTORY','DB_HOST','DB_PORT','DB_NAME','DB_USER','DB_PASSWORD','GAMEPLAY_MODE','V2_MIGRATION_CONFIRM','V2_BACKUP_CONFIRM','R5_INTEGRATION','R5_CONFIG','R5_ADMIN_CONFIG','R5_ACCEPTANCE_FAULT','GOOS','GOARCH','CGO_ENABLED','NUGET_PACKAGES','DOTNET_CLI_HOME','DOTNET_CLI_TELEMETRY_OPTOUT','DOTNET_GENERATE_ASPNET_CERTIFICATE')
$previous = @{}
foreach ($name in $variables) { $previous[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }
function New-Secret {
    $bytes = New-Object byte[] 32
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    return ([BitConverter]::ToString($bytes)).Replace('-','').ToLowerInvariant()
}
function Assert-Exit([string]$Label) { if ($LASTEXITCODE -ne 0) { throw "$Label failed." } }
function Wait-Until([string]$Label,[scriptblock]$Condition,[int]$Seconds=30) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        if (& $Condition) { return }
        Start-Sleep -Milliseconds 200
    }
    throw "$Label did not finish before its deadline."
}
function Read-SQL([string]$Query) {
    $result = $Query | docker exec -i gm-r5-verify-mysql-1 sh -lc 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -N -B -uroot'
    Assert-Exit 'Isolated SQL query'
    return $result
}
function Start-Role([string]$Role) {
    $binary = Join-Path $evidence 'bin/r5.exe'
    $process = Start-Process -FilePath $binary -ArgumentList @('-mode',$Role,'-config', ('"'+$runtimeConfig+'"')) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $evidence "$Role.stdout.log") -RedirectStandardError (Join-Path $evidence "$Role.stderr.log")
    $roles[$Role] = $process
}
function Stop-Role([string]$Role) {
    if ($roles.ContainsKey($Role)) {
        $process = $roles[$Role]
        if (-not $process.HasExited) { Stop-Process -Id $process.Id; $process.WaitForExit(5000) | Out-Null }
        $roles.Remove($Role)
    }
}
function Verify-Main([string]$Label) {
    docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode scenario
    Assert-Exit "$Label main scenario"
    docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode check
    Assert-Exit "$Label asset reconciliation"
}
try {
    docker info --format '{{.ServerVersion}}' *> $null
    if ($LASTEXITCODE -ne 0) { docker desktop start; Assert-Exit 'Docker startup' }
    $existing = docker ps -a --filter "label=com.docker.compose.project=$projectName" --format '{{.ID}}'
    Assert-Exit 'Docker availability'
    if ($existing) { throw 'gm-r5-verify already exists; inspect its owner before reusing it.' }
    foreach ($port in @(23306,26379,25672,25673,8080,18090,18091,18092,18093,18094)) {
        if (Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue) { throw "Port $port is already used." }
    }
    New-Item -ItemType Directory -Force -Path (Join-Path $evidence 'bin') | Out-Null
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    icacls $evidence /inheritance:r /grant:r "${identity}:(OI)(CI)F" 'SYSTEM:(OI)(CI)F' | Out-Null
    Assert-Exit 'Private evidence directory permissions'
    $env:PVE_TEST_PASSWORD = New-Secret
    $env:R5_ADMIN_PASSWORD = New-Secret
    $env:R5_MAIN_JWT = New-Secret
    $env:R5_MAIN_EVENT_TOKEN = New-Secret
    $env:R5_MAIN_PLAYER_PASSWORD = New-Secret
    $env:R5_BINARY_DIRECTORY = Join-Path $evidence 'bin'
    $composeSecrets = @('PVE_TEST_PASSWORD','R5_ADMIN_PASSWORD','R5_MAIN_JWT','R5_MAIN_EVENT_TOKEN','R5_MAIN_PLAYER_PASSWORD','R5_BINARY_DIRECTORY')
    $envLines = foreach ($name in $composeSecrets) { $value = [Environment]::GetEnvironmentVariable($name,'Process'); if ($name -eq 'R5_BINARY_DIRECTORY') { $value = $value.Replace('\','/') }; "$name=$value" }
    [IO.File]::WriteAllLines((Join-Path $evidence 'compose.env'),$envLines,[Text.UTF8Encoding]::new($false))
    $env:DB_HOST = '127.0.0.1'
    $env:DB_PORT = '23306'
    $env:DB_NAME = 'game_realtime_v2_test'
    $env:DB_USER = 'pve_test'
    $env:DB_PASSWORD = $env:PVE_TEST_PASSWORD
    $env:GAMEPLAY_MODE = 'v2'
    $env:V2_MIGRATION_CONFIRM = 'I_UNDERSTAND_V2_MIGRATION'
    $env:V2_BACKUP_CONFIRM = 'I_UNDERSTAND_V2_BACKUP'
    $env:NUGET_PACKAGES = Join-Path $EvidenceRoot 'nuget-packages'
    $env:DOTNET_CLI_HOME = Join-Path $EvidenceRoot 'dotnet-home'
    $env:DOTNET_CLI_TELEMETRY_OPTOUT = '1'
    $env:DOTNET_GENERATE_ASPNET_CERTIFICATE = 'false'
    Push-Location $moduleRoot
    try {
        go test ./...; Assert-Exit 'R5 unit tests'
        go vet ./...; Assert-Exit 'R5 vet'
        go build -o (Join-Path $evidence 'bin/r5.exe') ./cmd/r5; Assert-Exit 'R5 role binary'
        go build -o (Join-Path $evidence 'bin/query.exe') ./cmd/query; Assert-Exit 'R5 client binary'
        go build -o (Join-Path $evidence 'bin/prepare.exe') ./cmd/prepare; Assert-Exit 'R5 provisioning binary'
        dotnet run --project gen/csharp/ContractCheck.csproj -c Release --no-launch-profile; Assert-Exit 'C# contract'
    } finally { Pop-Location }
    Push-Location $backendRoot
    try {
        go test ./...; Assert-Exit 'Main backend regression'
        go vet ./...; Assert-Exit 'Main backend vet'
        go build -o (Join-Path $evidence 'bin/migrate.exe') ./cmd/tools/v2_migrate; Assert-Exit 'Migration tool'
        go build -o (Join-Path $evidence 'bin/backup.exe') ./cmd/tools/v2_backup; Assert-Exit 'Backup tool'
        $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
        go build -o (Join-Path $evidence 'bin/main-linux') ./cmd/server; Assert-Exit 'Isolated main server'
        go build -o (Join-Path $evidence 'bin/verify-linux') ./cmd/tools/pve_verify; Assert-Exit 'Main control-plane verifier'
    } finally {
        foreach ($name in @('GOOS','GOARCH','CGO_ENABLED')) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
        Pop-Location
    }
    $composeStarted = $true
    docker compose -f $compose up -d --wait --wait-timeout 180 mysql redis rabbitmq
    Assert-Exit 'Isolated dependencies'
    Push-Location $backendRoot
    try { & (Join-Path $evidence 'bin/migrate.exe') -stage r4; Assert-Exit 'Isolated R4 migration' } finally { Pop-Location }
    docker compose -f $compose --profile main up -d main; Assert-Exit 'Isolated main fixture'
    Wait-Until 'Main HTTP readiness' { try { (Invoke-WebRequest http://127.0.0.1:8080/health -TimeoutSec 2).StatusCode -eq 200 } catch { $false } }
    docker exec gm-r5-verify-main-1 /r5/bin/verify-linux -mode prepare; Assert-Exit 'Four synthetic accounts'
    Verify-Main 'baseline'
    $backup = Join-Path $evidence 'source.dump'
    & (Join-Path $evidence 'bin/backup.exe') -action backup -container gm-r5-verify-mysql-1 -database game_realtime_v2_test -file $backup; Assert-Exit 'Consistent fixture backup'
    $backupHash = (Get-FileHash $backup -Algorithm SHA256).Hash
    & (Join-Path $evidence 'bin/backup.exe') -action restore -container gm-r5-verify-mysql-1 -database game_realtime_v2_r5_restore -file $backup; Assert-Exit 'Isolated fixture restore'
    $adminConfig = Join-Path $evidence 'admin.json'
    $runtimeConfig = Join-Path $evidence 'runtime.json'
    $admin = @{root_db=@{address='127.0.0.1:23306';name='game_realtime_v2_r5_restore';user='root';password=$env:PVE_TEST_PASSWORD};broker=@{address='127.0.0.1:25672';vhost='r5';user='r5_admin';password=$env:R5_ADMIN_PASSWORD};management_address='127.0.0.1:25673'}
    [IO.File]::WriteAllText($adminConfig,($admin | ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
    Push-Location $moduleRoot
    try { & (Join-Path $evidence 'bin/prepare.exe') -admin $adminConfig -output $runtimeConfig; Assert-Exit 'Least-privilege provisioning' } finally { Pop-Location }
    $env:R5_CONFIG = $runtimeConfig; $env:R5_ADMIN_CONFIG = $adminConfig; $env:R5_INTEGRATION = '1'
    Push-Location $moduleRoot
    try { go test -count=1 -v -timeout 180s ./internal/experiment | Tee-Object -FilePath (Join-Path $evidence 'integration.log'); Assert-Exit 'Real MySQL and RabbitMQ integration' } finally { Pop-Location }
    $linuxCache = Join-Path $EvidenceRoot 'go-build-linux'
    $moduleCache = (go env GOMODCACHE).Trim()
    New-Item -ItemType Directory -Force -Path $linuxCache | Out-Null
    docker run --rm --name gm-r5-race --network gm-r5-verify_default -v "${moduleRoot}:/work:ro" -v "${evidence}:/evidence:ro" -v "${moduleCache}:/gomodcache" -v "${linuxCache}:/gocache" -w /work -e GOMODCACHE=/gomodcache -e GOCACHE=/gocache -e GOPROXY=https://proxy.golang.org,direct -e R5_INTEGRATION=1 -e R5_CONFIG=/evidence/runtime.json -e R5_ADMIN_CONFIG=/evidence/admin.json -e R5_TEST_NETWORK=compose golang:1.27.1-bookworm go test -race -count=1 -timeout 180s ./... | Tee-Object -FilePath (Join-Path $evidence 'race.log')
    Assert-Exit 'Linux race with real MySQL and RabbitMQ'
    Read-SQL 'DELETE FROM game_realtime_r5_reports.r5_report_runs; DELETE FROM game_realtime_r5_reports.r5_receipts; DELETE FROM game_realtime_r5_reports.r5_deliveries; DELETE FROM game_realtime_r5_reports.r5_scan_state;' | Out-Null
    Start-Role 'rpc'; Start-Role 'bridge'; Start-Role 'publisher'
    $env:R5_ACCEPTANCE_FAULT = 'after_commit'
    Start-Role 'consumer'
    $env:R5_ACCEPTANCE_FAULT = ''
    Wait-Until 'Actual consumer crash after commit' { $roles['consumer'].Refresh(); $roles['consumer'].HasExited }
    if ($roles['consumer'].ExitCode -ne 42) { throw 'Consumer failed before the intended commit/ACK fault.' }
    Copy-Item -LiteralPath (Join-Path $evidence 'consumer.stdout.log') -Destination (Join-Path $evidence 'consumer-crash.stdout.log')
    Stop-Role 'consumer'
    Start-Role 'consumer'
    Wait-Until 'ACK recovery and projection' { [int](Read-SQL "SELECT COUNT(*) FROM game_realtime_r5_reports.r5_receipts WHERE status='applied';") -ge 1 }
    Start-Sleep -Seconds 1
    & (Join-Path $evidence 'bin/query.exe') -config $runtimeConfig -clients (Join-Path $evidence 'clients.json'); Assert-Exit 'Independent real RPC client'
    $clients = Get-Content (Join-Path $evidence 'clients.json') -Raw | ConvertFrom-Json
    $headers = @{'X-Service-ID'='r5-observer';Authorization=('Bearer '+$clients.'r5-observer')}
    $metrics = (Invoke-WebRequest http://127.0.0.1:18094/metrics -Headers $headers -TimeoutSec 5).Content
    if ($metrics -notmatch 'r5_duplicates_total [1-9]') { throw 'Crash redelivery was not deduplicated.' }
    Verify-Main 'R5 workers active'
    Stop-Role 'rpc'
    Verify-Main 'RPC stopped'
    Stop-Role 'consumer'
    Read-SQL "UPDATE game_realtime_r5_reports.r5_deliveries SET status='pending',attempts=0,next_attempt_at=UTC_TIMESTAMP(3);" | Out-Null
    Wait-Until 'Durable queue backlog' { $statistics = docker exec gm-r5-verify-rabbitmq-1 rabbitmqctl -q list_queues -p r5 name messages_ready; Assert-Exit 'Backlog query'; [bool]($statistics -match 'r5.reports\s+[1-9]') }
    Verify-Main 'Consumer stopped'
    docker compose -f $compose stop rabbitmq; Assert-Exit 'Broker fault injection'
    Verify-Main 'Broker stopped'
    docker compose -f $compose up -d --wait --wait-timeout 120 rabbitmq; Assert-Exit 'Broker restart'
    $statistics = docker exec gm-r5-verify-rabbitmq-1 rabbitmqctl -q list_queues -p r5 name messages_ready
    Assert-Exit 'Persisted backlog query'
    if (-not ($statistics -match 'r5.reports\s+[1-9]')) { throw 'Broker restart lost confirmed backlog.' }
    Start-Role 'consumer'; Start-Role 'rpc'
    Wait-Until 'RPC service restart' { & (Join-Path $evidence 'bin/query.exe') -config $runtimeConfig -clients (Join-Path $evidence 'clients.json') *> $null; $LASTEXITCODE -eq 0 }
    docker compose -f $compose stop main; Assert-Exit 'Stop primary fixture for independent query'
    & (Join-Path $evidence 'bin/query.exe') -config $runtimeConfig -clients (Join-Path $evidence 'clients.json'); Assert-Exit 'RPC independent from primary service'
    docker compose -f $compose stop mysql; Assert-Exit 'Source database fault injection'
    & (Join-Path $evidence 'bin/query.exe') -config $runtimeConfig -clients (Join-Path $evidence 'clients.json') *> $null
    if ($LASTEXITCODE -eq 0) { throw 'Unavailable database returned fabricated query data.' }
    docker compose -f $compose up -d --wait --wait-timeout 120 mysql; Assert-Exit 'Source database restart'
    Wait-Until 'Database client recovery' { & (Join-Path $evidence 'bin/query.exe') -config $runtimeConfig -clients (Join-Path $evidence 'clients.json') *> $null; $LASTEXITCODE -eq 0 }
    foreach ($role in @('rpc','bridge','publisher','consumer')) { Stop-Role $role }
    docker compose -f $compose --profile main up -d main; Assert-Exit 'Primary fixture rollback'
    Wait-Until 'Primary recovery' { try { (Invoke-WebRequest http://127.0.0.1:8080/health -TimeoutSec 2).StatusCode -eq 200 } catch { $false } }
    Verify-Main 'All experiments removed'
    $summary = @{date=(Get-Date -Format 'yyyy-MM-dd');status='PASS';backup_sha256=$backupHash;scope='R5 local isolated RPC/MQ experiment';source='restored real four-player V2 transactional outbox';checks=@('contracts','service identity','deadline','read-only permissions','publisher confirms','manual ACK','bounded retry and dead-letter','duplicate and conflict','actual process crash before ACK','broker restart and backlog','projection rebuild','RPC and database recovery','primary query and asset isolation','experiment rollback','real storage Linux race')}
    $summary | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $evidence 'summary.json') -Encoding utf8
    Write-Output "R5 acceptance PASS. Private evidence: $evidence"
} finally {
    foreach ($role in @($roles.Keys)) { Stop-Role $role }
    if ($composeStarted -and -not $KeepEnvironment) {
        docker compose -f $compose --profile main down -v; Assert-Exit 'R5 isolated resource cleanup'
        foreach ($name in @('compose.env','admin.json','runtime.json','clients.json')) {
            $secretPath = Join-Path $evidence $name
            if (Test-Path -LiteralPath $secretPath) { Remove-Item -LiteralPath $secretPath }
        }
    }
    foreach ($name in $variables) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
}
