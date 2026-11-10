# Bounded launcher models. Docker and Go are replaced by in-process fakes.
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-common.ps1')
$testRoot=Join-Path ([IO.Path]::GetTempPath()) ('gm-local-model-'+[Guid]::NewGuid().ToString('N'))
$testCases=[Collections.Generic.List[string]]::new()
$global:localModelMode='empty'
$global:localModelState=$null
$global:localModelCalls=[Collections.Generic.List[string]]::new()
$global:localModelGoCalls=0;$global:localModelGoFail=$false;$global:localModelGoExplicit=$false
$global:localModelCommitMB=8192;$global:localModelPhysicalMB=8192;$global:localModelMemoryUnavailable=$false
function global:Get-CimInstance {
    if($global:localModelMemoryUnavailable){throw 'Unavailable model counters.'}
    [pscustomobject]@{FreeVirtualMemory=$global:localModelCommitMB*1024;FreePhysicalMemory=$global:localModelPhysicalMB*1024}
}
function New-LocalModelProcess([int]$ExitCode=0,[int]$ExitAfterWaits=1){
    $process=[pscustomobject]@{HasExited=$false;ExitCode=$ExitCode;waits=0;exit_after=$ExitAfterWaits;killed=$false;disposed=$false;StandardOutput=[IO.StringReader]::new("Fake owned process output.`n");StandardError=[IO.StringReader]::new('')}
    $process|Add-Member ScriptMethod WaitForExit {param($milliseconds);$this.waits++;if($this.waits -ge $this.exit_after){$this.HasExited=$true};return $this.HasExited}
    $process|Add-Member ScriptMethod Kill {param($entireTree);if(!$entireTree){throw 'Owned process cleanup must cover its own children.'};$this.killed=$true;$this.HasExited=$true}
    $process|Add-Member ScriptMethod Dispose {$this.disposed=$true;$this.StandardOutput.Dispose();$this.StandardError.Dispose()}
    $process
}
function Start-LocalOwnedProcess([string]$Executable,[string[]]$Arguments){
    if($Arguments -notcontains './cmd/tools/local_check' -or $Arguments -contains './cmd/tools/v2_migrate'){throw 'Existing Check invoked a writing tool.'}
    $global:localModelGoCalls++
    $global:localModelGoExplicit=$env:DB_NAME -eq 'game_realtime' -and $env:DB_PASSWORD -ne 'caller-value-not-a-secret' -and !$env:PVE_TEST_EVENTS_ENABLED -and !$env:DC_GAME_ENABLED
    New-LocalModelProcess -ExitCode $(if($global:localModelGoFail){9}else{0})
}
function Invoke-LocalModelEntry {
    # Execute the unchanged entry logic against the functions loaded above.
    # Only its repeated dot-source is omitted so mocks remain in this test scope.
    $source=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'local.ps1'))
    $source=$source.Replace(". (Join-Path `$PSScriptRoot 'local-common.ps1')",'')
    & ([scriptblock]::Create($source)) @args
}
function global:docker {
    $arguments=@($args);$global:LASTEXITCODE=0
    $global:localModelCalls.Add(($arguments[0..([Math]::Min(2,$arguments.Count-1))] -join ' '))
    if($arguments[0] -eq 'ps'){
        if($arguments -notcontains ('label=com.docker.compose.project=gm-local-'+$script:localContext.identity)){throw 'Unscoped container listing.'}
        if($global:localModelMode -in @('owned','wrong-owner')){return @('111111111111','222222222222')}
        return
    }
    if($arguments[0] -eq 'volume' -and $arguments[1] -eq 'ls'){
        if($arguments -notcontains ('label=com.docker.compose.project=gm-local-'+$script:localContext.identity)){throw 'Unscoped volume listing.'}
        if($global:localModelMode -in @('owned','wrong-owner','orphan')){return @(($global:localModelState.compose_project+'_mysql-data'),($global:localModelState.compose_project+'_redis-data'))}
        return
    }
    if($arguments[0] -eq 'inspect'){
        $owner=if($global:localModelMode -eq 'wrong-owner'){'another-owner'}else{$global:localModelState.owner}
        return (@{owner=$owner;project=$global:localModelState.compose_project;service=$(if($arguments[-1] -eq '111111111111'){'mysql'}else{'redis'});running=$true}|ConvertTo-Json -Compress)
    }
    if($arguments[0] -eq 'volume' -and $arguments[1] -eq 'inspect'){
        return (@{owner=$global:localModelState.owner;project=$global:localModelState.compose_project}|ConvertTo-Json -Compress)
    }
    if($arguments[0] -eq 'compose'){
        if($arguments -notcontains '--project-name' -or $arguments -notcontains $global:localModelState.compose_project){throw 'Unscoped Compose action.'}
        if($arguments -contains 'down' -or $arguments -contains '--volumes'){throw 'Destructive storage operation was requested.'}
        if($arguments -contains 'stop'){$script:localStopped=$true;return}
        throw 'Unexpected Compose mutation in model.'
    }
    throw 'Unexpected Docker model command.'
}
function Assert-LocalFreePorts([int[]]$Ports,[switch]$AllowOwned){if($script:localBlockPorts){throw 'Port occupied model.'}}
function Assert-Model([bool]$Condition,[string]$Name){if(!$Condition){throw ('Model assertion failed: '+$Name)};$testCases.Add($Name)}
function Assert-ModelReject([scriptblock]$Action,[string]$Name){$rejected=$false;try{& $Action|Out-Null}catch{$rejected=$true};Assert-Model $rejected $Name}
$localBlockPorts=$false;$localStopped=$false
$originalPassword=$env:DB_PASSWORD;$originalTestFlag=$env:PVE_TEST_EVENTS_ENABLED
try{
    [void][IO.Directory]::CreateDirectory($testRoot)
    $localContext=Get-LocalContext -StateRoot (Join-Path $testRoot 'owned state')
    Initialize-LocalInstallation $localContext|Out-Null
    $state=Read-LocalInstallation $localContext
    $global:localModelState=$state
    Assert-Model ($state.http_port -eq 18080 -and $state.mysql_port -eq 33306 -and $state.redis_port -eq 36379) 'isolated ports do not reuse legacy or Dungeon Contract ports'
    $before=@{};foreach($file in Get-ChildItem -LiteralPath $localContext.private -File){$before[$file.Name]=(Get-FileHash -LiteralPath $file.FullName).Hash}
    Initialize-LocalInstallation $localContext|Out-Null
    foreach($file in Get-ChildItem -LiteralPath $localContext.private -File){Assert-Model ((Get-FileHash -LiteralPath $file.FullName).Hash -eq $before[$file.Name]) ('repeat prepare preserves '+$file.Name)}
    $global:localModelMode='owned'
    Assert-LocalResources $localContext $state -RequireRunning
    Assert-Model $true 'owned containers and volumes are verified through labels'
    Invoke-LocalCompose $localContext $state @('stop','--timeout','30')|Out-Null
    Assert-Model $localStopped 'stop is scoped to the verified project and retains volumes'
    $global:localModelMode='wrong-owner'
    Assert-ModelReject {Assert-LocalResources $localContext $state -RequireRunning} 'foreign container owner is rejected before mutation'
    $global:localModelMode='orphan'
    Assert-ModelReject {Assert-LocalResources $localContext $state} 'orphaned existing volumes cannot be adopted'
    $global:localModelMode='empty'
    [IO.File]::AppendAllText((Join-Path $localContext.private 'db-password.txt'),'changed')
    Assert-ModelReject {Read-LocalInstallation $localContext} 'changed existing credential cannot be silently regenerated'
    Assert-ModelReject {Initialize-LocalInstallation $localContext} 'repeat prepare preserves a failed credential identity'
    $partial=Get-LocalContext -StateRoot (Join-Path $testRoot 'partial')
    [void][IO.Directory]::CreateDirectory($partial.root);[IO.File]::WriteAllText((Join-Path $partial.root 'unknown.txt'),'keep')
    Assert-ModelReject {Initialize-LocalInstallation $partial} 'unrecognized state is not replaced'
    Assert-ModelReject {Assert-LocalExternalPath (Join-Path $localContext.project 'private') $localContext.project} 'source-tree credentials are rejected'
    $portContext=Get-LocalContext -StateRoot (Join-Path $testRoot 'occupied')
    $localBlockPorts=$true
    Assert-ModelReject {Initialize-LocalInstallation $portContext} 'port collision refuses preparation without changing ports'
    Assert-Model (!(Test-Path -LiteralPath $portContext.root)) 'port collision creates no credentials or state'
    $localBlockPorts=$false
    $existingRoot=Join-Path $testRoot 'existing private'
    [void][IO.Directory]::CreateDirectory($existingRoot);Set-LocalPrivatePermissions $existingRoot
    foreach($name in @('db','jwt')){[IO.File]::WriteAllText((Join-Path $existingRoot $name),([string]$name)*40);Set-LocalSecretFilePermissions (Join-Path $existingRoot $name)}
    $existing=[pscustomobject]@{schema_version=1;environment=[pscustomobject]@{APP_HOST='127.0.0.1';APP_PORT='8080';GAMEPLAY_MODE='v2';DB_HOST='127.0.0.1';DB_PORT='3306';DB_NAME='game_realtime';DB_USER='game';DB_PASSWORD_FILE=(Join-Path $existingRoot 'db');REDIS_ADDR='127.0.0.1:6379';REDIS_DB='0';REDIS_PASSWORD_FILE='';JWT_SECRET_FILE=(Join-Path $existingRoot 'jwt')}}
    $existingPath=Join-Path $existingRoot 'existing.json';Write-LocalJson $existingPath $existing
    $calls=$global:localModelCalls.Count
    $values=Read-LocalExistingEnvironment $localContext $existingPath
    Assert-Model ($values.DB_NAME -eq 'game_realtime' -and $values.DB_PORT -eq '3306' -and $values.REDIS_PASSWORD -eq '') 'explicit existing legacy storage preserves its address and no-auth Redis'
    Assert-Model ($calls -eq $global:localModelCalls.Count) 'reading Existing configuration performs no Docker action'
    $env:DB_PASSWORD='caller-value-not-a-secret';$env:PVE_TEST_EVENTS_ENABLED='true'
    $untouched=Join-Path $testRoot 'existing-unused-state'
    Invoke-LocalModelEntry -Profile Existing -Action Check -StateRoot $untouched -ConfigPath $existingPath|Out-Null
    Assert-Model ($global:localModelGoCalls -eq 1 -and $global:localModelGoExplicit) 'Existing Check calls only the bounded read-only tool with explicit sanitized environment'
    Assert-Model (!(Test-Path -LiteralPath $untouched) -and $calls -eq $global:localModelCalls.Count) 'Existing Check creates no state and invokes no Docker operations'
    Assert-Model ($env:DB_PASSWORD -eq 'caller-value-not-a-secret' -and $env:PVE_TEST_EVENTS_ENABLED -eq 'true') 'Existing Check restores caller environment'
    $global:localModelGoFail=$true
    Assert-ModelReject {Invoke-LocalModelEntry -Profile Existing -Action Check -StateRoot $untouched -ConfigPath $existingPath} 'failed Existing Check remains a failure'
    Assert-Model ($env:DB_PASSWORD -eq 'caller-value-not-a-secret' -and $env:PVE_TEST_EVENTS_ENABLED -eq 'true') 'failed Existing Check also restores caller environment'
    $global:localModelGoFail=$false
    if($IsWindows){
        $beforeGo=$global:localModelGoCalls
        $global:localModelCommitMB=1791
        Assert-ModelReject {Invoke-LocalModelEntry -Profile Existing -Action Check -StateRoot $untouched -ConfigPath $existingPath} 'Go admission rejects below 1792 MiB commit before creating a process'
        Assert-Model ($beforeGo -eq $global:localModelGoCalls) 'rejected memory admission starts no Go process'
        $global:localModelCommitMB=2687;$global:localModelPhysicalMB=1536
        Assert-ModelReject {Assert-LocalMemoryAdmission ServicesStart} 'storage startup rejects below 2688 MiB commit'
        $global:localModelCommitMB=8192;$global:localModelPhysicalMB=1023
        Assert-ModelReject {Assert-LocalMemoryAdmission Go} 'Go admission rejects below 1024 MiB physical'
        $global:localModelCommitMB=1792;$global:localModelPhysicalMB=1024
        Assert-LocalMemoryAdmission Go
        $global:localModelCommitMB=2688;$global:localModelPhysicalMB=1536
        Assert-LocalMemoryAdmission ServicesStart
        Assert-Model $true 'exact Go and storage admission thresholds are preserved'
        $global:localModelMemoryUnavailable=$true
        Assert-ModelReject {Assert-LocalMemoryAdmission Go} 'unavailable Windows memory counters fail closed'
        $global:localModelMemoryUnavailable=$false
        $global:localModelCommitMB=1023;$global:localModelPhysicalMB=8192
        $guarded=New-LocalModelProcess -ExitAfterWaits 100
        Assert-ModelReject {Wait-LocalOwnedProcess $guarded} 'running owned process stops below the unchanged 1 GiB commit reserve'
        Assert-Model ($guarded.killed -and $guarded.disposed) 'guard failure cleans only the observed owned process tree'
        $global:localModelCommitMB=8192;$global:localModelPhysicalMB=511
        Assert-ModelReject {Assert-LocalMemoryReserve} 'running reserve also rejects below 512 MiB physical'
        $global:localModelCommitMB=8192;$global:localModelPhysicalMB=8192
    }
    function Test-LocalReadiness([string]$Address){return $true}
    $readyProcess=New-LocalModelProcess -ExitAfterWaits 2
    Wait-LocalOwnedProcess $readyProcess -ReadyAddress 'http://127.0.0.1:18080/ready'|Out-Null
    Assert-Model ($readyProcess.disposed -and !$readyProcess.killed) 'readiness is confirmed while the owned server runs'
    function Test-LocalReadiness([string]$Address){return $false}
    $earlyExit=New-LocalModelProcess
    Assert-ModelReject {Wait-LocalOwnedProcess $earlyExit -ReadyAddress 'http://127.0.0.1:18080/ready'} 'server exit before readiness remains a failure'
    $existing.environment|Add-Member -NotePropertyName Execute -NotePropertyValue 'throw malicious-config'
    Write-LocalJson $existingPath $existing
    Assert-ModelReject {Read-LocalExistingEnvironment $localContext $existingPath} 'unknown executable-style configuration keys are rejected without evaluation'
    $existing.environment.PSObject.Properties.Remove('Execute');$existing.environment.DB_NAME='game_realtime_dc_demo';Write-LocalJson $existingPath $existing
    Assert-ModelReject {Read-LocalExistingEnvironment $localContext $existingPath} 'Existing refuses the separate Dungeon Contract database'
    foreach($action in @('Prepare','ServicesStart','ServicesStop','Initialize','CreateAdmin')){
        Assert-ModelReject {Invoke-LocalModelEntry -Profile Existing -Action $action -ConfigPath $existingPath} ('Existing refuses '+$action)
    }
    [pscustomobject]@{status='PASS_LOCAL_LAUNCHER_MODELS';cases=$testCases.Count;checks=$testCases.ToArray();docker_started=$false;go_compiled=$false;credentials_printed=$false}|ConvertTo-Json -Depth 5
}finally{
    Remove-Item Function:/docker -ErrorAction SilentlyContinue
    Remove-Item Function:/Get-CimInstance -ErrorAction SilentlyContinue
    $env:DB_PASSWORD=$originalPassword;$env:PVE_TEST_EVENTS_ENABLED=$originalTestFlag
    if(Test-Path -LiteralPath $testRoot){
        $checked=[IO.Path]::GetFullPath($testRoot)
        $parent=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar
        if(!$checked.StartsWith($parent,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($checked) -notmatch '^gm-local-model-[a-f0-9]{32}$'){throw 'Model cleanup boundary mismatch.'}
        Remove-Item -LiteralPath $checked -Recurse -Force
    }
}
