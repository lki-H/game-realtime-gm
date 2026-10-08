param(
    [Parameter(Mandatory = $true)][string]$RollbackBackendPath
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$backendRoot = Join-Path $projectRoot 'backend'
$rollbackRoot = (Resolve-Path -LiteralPath $RollbackBackendPath).Path
if (-not (Test-Path -LiteralPath (Join-Path $rollbackRoot 'cmd/server/main.go'))) {
    throw 'RollbackBackendPath must point to the reviewed R3 backend source.'
}
$composePath = Join-Path $PSScriptRoot 'docker-compose.v2-test.yml'
$projectName = 'gm-r4-verify'
$existing = docker ps -a --filter "label=com.docker.compose.project=$projectName" --format '{{.ID}}'
if ($LASTEXITCODE -ne 0 -or $existing) {
    throw 'Docker must be available and gm-r4-verify must not already exist.'
}
$artifactDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ('gm-r4-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $artifactDirectory | Out-Null
$currentBinary = Join-Path $artifactDirectory 'pve-r4.exe'
$rollbackBinary = Join-Path $artifactDirectory 'pve-r3.exe'
$variableNames = @('PVE_TEST_PASSWORD', 'PVE_INTEGRATION', 'PVE_TEST_NETWORK', 'PVE_R4_SERVER_BINARY', 'PVE_R3_SERVER_BINARY', 'GIN_MODE')
$previous = @{}
foreach ($name in $variableNames) {
    $previous[$name] = [System.Environment]::GetEnvironmentVariable($name, 'Process')
}
$composeStarted = $false
try {
    $randomBytes = New-Object byte[] 32
    $generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($randomBytes) } finally { $generator.Dispose() }
    $env:PVE_TEST_PASSWORD = [Convert]::ToBase64String($randomBytes)
    $env:PVE_INTEGRATION = '1'
    $env:PVE_TEST_NETWORK = ''
    $env:PVE_R4_SERVER_BINARY = $currentBinary
    $env:PVE_R3_SERVER_BINARY = $rollbackBinary
    $env:GIN_MODE = 'release'
    Push-Location $backendRoot
    try {
        go build -o $currentBinary ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'R4 build failed.' }
    } finally { Pop-Location }
    Push-Location $rollbackRoot
    try {
        go build -o $rollbackBinary ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'R3 baseline build failed.' }
    } finally { Pop-Location }
    $composeStarted = $true
    docker compose -p $projectName -f $composePath up -d --wait --wait-timeout 150
    if ($LASTEXITCODE -ne 0) { throw 'Isolated environment startup failed.' }
    Push-Location $backendRoot
    try {
        go test -count=1 -timeout 240s ./tests/v2
        if ($LASTEXITCODE -ne 0) { throw 'V2 integration or binary rollback failed.' }
        go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    } finally { Pop-Location }
} finally {
    $cleanupFailure = $null
    if ($composeStarted) {
        docker compose -p $projectName -f $composePath down -v
        if ($LASTEXITCODE -ne 0) { $cleanupFailure = 'Test cleanup failed; inspect gm-r4-verify resources.' }
    }
    foreach ($name in $variableNames) {
        [System.Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
    foreach ($binaryPath in @($currentBinary, $rollbackBinary)) {
        if (Test-Path -LiteralPath $binaryPath) { Remove-Item -LiteralPath $binaryPath }
    }
    if (Test-Path -LiteralPath $artifactDirectory) { Remove-Item -LiteralPath $artifactDirectory }
    if ($cleanupFailure) { throw $cleanupFailure }
}
