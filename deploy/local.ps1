param(
    [ValidateSet('Isolated','Existing')][string]$Profile='Isolated',
    [ValidateSet('Prepare','Check','ServicesStart','ServicesStop','Initialize','CreateAdmin','Server')][string]$Action='Check',
    [string]$StateRoot='',
    [string]$ConfigPath='',
    [string]$Username='',
    [string]$PasswordFile=''
)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion -lt [Version]'7.4'){throw 'PowerShell 7.4 or newer is required.'}
. (Join-Path $PSScriptRoot 'local-common.ps1')
if($Profile -eq 'Existing' -and $Action -notin @('Check','Server')){throw 'Existing supports only read-only Check and explicitly configured Server; storage and credentials are never managed.'}
$context=Get-LocalContext -StateRoot $StateRoot
$localMutex=[Threading.Mutex]::new($false,('Local\GameRealtimeGMLocal-'+$context.identity))
$localLocked=$false
$saved=@{}
try{
    try{$localLocked=$localMutex.WaitOne(0)}catch [Threading.AbandonedMutexException]{$localLocked=$true}
    if(!$localLocked){throw 'Another local launcher action for this checkout is running.'}
    if($Profile -eq 'Isolated'){
        if($ConfigPath){throw 'Isolated uses its owned installation record; ConfigPath is only for Existing.'}
        if($Action -eq 'Prepare'){Initialize-LocalInstallation $context;return}
        $state=Read-LocalInstallation $context
        Assert-LocalResources $context $state
        $values=Get-LocalIsolatedEnvironment $context $state
    }else{
        if(!$ConfigPath){throw 'Existing requires an explicit external JSON ConfigPath.'}
        $values=Read-LocalExistingEnvironment $context $ConfigPath
    }
    $clear=@('PVE_INTEGRATION','PVE_TEST_PASSWORD','PVE_TEST_NETWORK','PVE_R4_SERVER_BINARY','PVE_R3_SERVER_BINARY','PVE_RULES_PATH','DC_GAME_ENABLED','DC_CONFIG_PATH','DC_ADMISSION_KEY','DC_SERVICE_TOKEN','GM_LOCAL_OWNED_ADMIN','V2_MIGRATION_CONFIRM','PVE_TEST_EVENTS_ENABLED','PVE_TEST_EVENTS_TOKEN','PVE_METRICS_ENABLED','PVE_METRICS_TOKEN','PVE_ARCHIVE_ENABLED','PPROF_ENABLED')
    foreach($name in $clear){$saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process');[Environment]::SetEnvironmentVariable($name,$null,'Process')}
    foreach($item in $values.GetEnumerator()){
        if(!$saved.ContainsKey($item.Key)){$saved[$item.Key]=[Environment]::GetEnvironmentVariable($item.Key,'Process')}
        [Environment]::SetEnvironmentVariable($item.Key,[string]$item.Value,'Process')
    }
    $backend=Join-Path $context.project 'backend'
    Push-Location -LiteralPath $backend
    try{
        switch($Action){
            'Check' {Invoke-LocalNative go @('run','-p','1','./cmd/tools/local_check','-expected-database',$values.DB_NAME)}
            'ServicesStart' {
                Assert-LocalMemoryAdmission ServicesStart
                Assert-LocalFreePorts @([int]$state.mysql_port,[int]$state.redis_port) -AllowOwned:$false
                Invoke-LocalCompose $context $state @('up','--detach','--wait','--wait-timeout','180')
                Assert-LocalResources $context $state -RequireRunning
            }
            'ServicesStop' {Invoke-LocalCompose $context $state @('stop','--timeout','30')}
            'Initialize' {
                Assert-LocalResources $context $state -RequireRunning
                if($state.initialization -ne 'NEW_EMPTY_INSTALLATION'){throw 'This installation has already begun initialization; use Check or explicitly inspect the retained failure.'}
                Invoke-LocalNative go @('run','-p','1','./cmd/tools/local_check','-mode','empty','-expected-database','game_realtime_local')
                $state.initialization='INITIALIZATION_STARTED'
                Write-LocalJson (Join-Path $context.root 'installation.json') $state
                # Stream the original schema without rewriting its bytes or
                # including credentials in an argument or output.
                Invoke-LocalSchema $context $state (Join-Path $backend 'internal/database/schema.sql')
                $env:V2_MIGRATION_CONFIRM='I_UNDERSTAND_V2_MIGRATION'
                Invoke-LocalNative go @('run','-p','1','./cmd/tools/v2_migrate','-stage','r4')
                Invoke-LocalNative go @('run','-p','1','./cmd/tools/local_check','-expected-database','game_realtime_local')
                $state.initialization='INITIALIZED'
                Write-LocalJson (Join-Path $context.root 'installation.json') $state
            }
            'CreateAdmin' {
                Assert-LocalResources $context $state -RequireRunning
                if($state.initialization -ne 'INITIALIZED'){throw 'Initialize this owned empty installation before creating an administrator.'}
                if(!$Username -or !$PasswordFile){throw 'CreateAdmin requires Username and an external protected PasswordFile; no default account is seeded.'}
                $passwordPath=Assert-LocalCredentialFile $PasswordFile $context.project
                $env:GM_LOCAL_OWNED_ADMIN='1'
                Invoke-LocalNative go @('run','-p','1','./cmd/tools/local_admin','-username',$Username,'-password-file',$passwordPath)
            }
            'Server' {
                if($Profile -eq 'Isolated'){Assert-LocalResources $context $state -RequireRunning}
                Assert-LocalFreePorts @([int]$values.APP_PORT) -AllowOwned:$false
                Invoke-LocalNative go @('run','-p','1','./cmd/tools/local_check','-expected-database',$values.DB_NAME)
                Write-Host ('Starting foreground V2 service at http://'+$values.APP_HOST+':'+$values.APP_PORT+'; stop this owned process with Ctrl+C.')
                $readyHost=if($values.APP_HOST -eq '::1'){'[::1]'}else{$values.APP_HOST}
                Invoke-LocalServer $context -ReadyAddress ('http://'+$readyHost+':'+$values.APP_PORT+'/ready')
            }
        }
    }finally{Pop-Location}
}finally{
    foreach($item in $saved.GetEnumerator()){[Environment]::SetEnvironmentVariable($item.Key,$item.Value,'Process')}
    if($localLocked){$localMutex.ReleaseMutex()};$localMutex.Dispose()
}
