param([Parameter(Mandatory=$true)][string]$EnvironmentDirectory,[switch]$ResetData)
$ErrorActionPreference = 'Stop'
$directory = (Resolve-Path -LiteralPath $EnvironmentDirectory).Path
$processRecord = Join-Path $directory 'processes.json'
$expectedBinary = [IO.Path]::GetFullPath((Join-Path $directory 'bin/r5.exe'))
if (Test-Path -LiteralPath $processRecord) {
    $records = @(Get-Content -LiteralPath $processRecord -Raw | ConvertFrom-Json)
    foreach ($record in $records) {
        $process = Get-Process -Id $record.process_id -ErrorAction SilentlyContinue
        if (-not $process) { continue }
        $recordedBinary = [IO.Path]::GetFullPath([string]$record.executable)
        $recordedStart = ([datetime]$record.started_at).ToUniversalTime()
        if ($process.Path -ne $expectedBinary -or $process.Path -ne $recordedBinary -or $process.StartTime.ToUniversalTime().Ticks -ne $recordedStart.Ticks) { throw 'Process identity changed; refusing to stop an unrelated process.' }
        Stop-Process -Id $process.Id
    }
    Remove-Item -LiteralPath $processRecord
}
$environment = Join-Path $directory 'compose.env'
if (-not (Test-Path -LiteralPath $environment)) { throw 'The isolated compose credential file is missing.' }
$composeArguments = @('compose','--env-file',$environment,'-f',(Join-Path $PSScriptRoot 'compose.yaml'),'--profile','main','down')
if ($ResetData) { $composeArguments += '--volumes' }
& docker @composeArguments
if ($LASTEXITCODE -ne 0) { throw 'R5 shutdown failed.' }
if ($ResetData) {
    foreach ($name in @('compose.env','admin.json','runtime.json','clients.json')) {
        $path = Join-Path $directory $name
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path }
    }
}
Write-Output 'R5 roles and isolated containers stopped. Docker Desktop remains under your control.'
