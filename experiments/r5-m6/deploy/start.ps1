param([Parameter(Mandatory=$true)][string]$EnvironmentDirectory)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$directory = (Resolve-Path -LiteralPath $EnvironmentDirectory).Path
$configuration = Join-Path $directory 'runtime.json'
$environment = Join-Path $directory 'compose.env'
$binary = [IO.Path]::GetFullPath((Join-Path $directory 'bin/r5.exe'))
foreach ($path in @($configuration,$environment,$binary)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Use verify.ps1 -KeepEnvironment to prepare an isolated runtime first.' }
}
$existing = Join-Path $directory 'processes.json'
if (Test-Path -LiteralPath $existing) { throw 'A process record already exists; stop the recorded runtime before starting it again.' }
docker info --format '{{.ServerVersion}}' *> $null
if ($LASTEXITCODE -ne 0) { docker desktop start; if ($LASTEXITCODE -ne 0) { throw 'Docker startup failed.' } }
docker compose --env-file $environment -f (Join-Path $PSScriptRoot 'compose.yaml') up -d --wait --wait-timeout 150 mysql redis rabbitmq
if ($LASTEXITCODE -ne 0) { throw 'Isolated dependency startup failed.' }
$records = @()
$oldFault = $env:R5_ACCEPTANCE_FAULT
$env:R5_ACCEPTANCE_FAULT = ''
try {
    foreach ($role in @('rpc','bridge','publisher','consumer')) {
        $process = Start-Process -FilePath $binary -ArgumentList @('-mode',$role,'-config',('"'+$configuration+'"')) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $directory "$role.stdout.log") -RedirectStandardError (Join-Path $directory "$role.stderr.log")
        $records += @{role=$role;process_id=$process.Id;started_at=$process.StartTime.ToUniversalTime().ToString('o');executable=$binary}
        $records | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $existing -Encoding utf8
    }
    Start-Sleep -Seconds 1
    foreach ($record in $records) {
        $process = Get-Process -Id $record.process_id -ErrorAction SilentlyContinue
        if (-not $process) { throw 'An R5 role exited during startup; inspect its local log.' }
    }
    Write-Output 'R5 roles started. RPC: 127.0.0.1:18090. Metrics: 18091-18094. All listeners are local.'
} catch {
    & (Join-Path $PSScriptRoot 'stop.ps1') -EnvironmentDirectory $directory
    throw
} finally { $env:R5_ACCEPTANCE_FAULT = $oldFault }
