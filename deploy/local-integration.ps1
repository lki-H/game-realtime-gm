param([string]$EvidenceRoot=$env:RUNNER_TEMP)
# Run only on a disposable Linux CI runner. This harness manages only the
# installation it creates; Existing Check uses that same synthetic storage.
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
if($PSVersionTable.PSVersion -lt [Version]'7.4' -or $IsWindows){throw 'This integration harness requires PowerShell 7.4+ on Linux.'}
if(!$EvidenceRoot -or !$env:CI){throw 'An explicit external evidence root and disposable CI environment are required.'}
. (Join-Path $PSScriptRoot 'local-common.ps1')
$project=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$parent=Assert-LocalExternalPath $EvidenceRoot $project
if(!(Test-Path -LiteralPath $parent -PathType Container)){throw 'The external evidence parent must already exist.'}
$evidence=Join-Path $parent ('gm-local-integration-'+[Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($evidence)
Set-LocalPrivatePermissions $evidence
$context=Get-LocalContext -StateRoot (Join-Path $evidence 'installation')
$entry=Join-Path $PSScriptRoot 'local.ps1'
$processes=[Collections.Generic.List[object]]::new()
$checks=[Collections.Generic.List[string]]::new()
$state=$null
$ownsInstallation=$false
$readOnlyWasSet=$false
$originalReadOnly=$null
$browserStatus='NOT_REQUESTED'
$success=$false
$saved=@{}
$integrationStage='prepare'
$containerStates=@()
$cleanupFailed=$false

function Assert-Integration([bool]$Condition,[string]$Name){
    if(!$Condition){throw ('Local integration assertion failed: '+$Name)}
    $checks.Add($Name)
}

function Start-IntegrationProcess([string]$Executable,[string[]]$Arguments,[string]$Label,[string]$Directory=$project,[hashtable]$Environment=@{}){
    $script:integrationStage=$Label
    $command=Get-Command $Executable -CommandType Application -ErrorAction Stop|Select-Object -First 1
    $start=[Diagnostics.ProcessStartInfo]::new($command.Source)
    foreach($argument in $Arguments){$start.ArgumentList.Add($argument)}
    $start.WorkingDirectory=$Directory
    $start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    foreach($item in $Environment.GetEnumerator()){$start.Environment[$item.Key]=[string]$item.Value}
    # Consume both pipes immediately. Only sanitized status reaches the job log;
    # raw child diagnostics remain inside this private directory.
    $stdout=[IO.File]::Open((Join-Path $evidence ($Label+'.stdout.log')),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::Read)
    $stderr=[IO.File]::Open((Join-Path $evidence ($Label+'.stderr.log')),[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::Read)
    foreach($path in @((Join-Path $evidence ($Label+'.stdout.log')),(Join-Path $evidence ($Label+'.stderr.log')))){Set-LocalSecretFilePermissions $path}
    try{$process=[Diagnostics.Process]::Start($start)}catch{$stdout.Dispose();$stderr.Dispose();throw 'Owned integration process could not start.'}
    $record=[pscustomobject]@{process=$process;stdout=$stdout;stderr=$stderr;output=$process.StandardOutput.BaseStream.CopyToAsync($stdout);errors=$process.StandardError.BaseStream.CopyToAsync($stderr);label=$Label;finished=$false}
    $processes.Add($record)
    $record
}

function Complete-IntegrationProcess($Record,[int]$TimeoutSeconds=240,[switch]$ExpectedFailure){
    if(!$Record.process.WaitForExit($TimeoutSeconds*1000)){
        $Record.process.Kill($true);[void]$Record.process.WaitForExit(10000)
        throw ('Owned integration process timed out: '+$Record.label)
    }
    $Record.output.GetAwaiter().GetResult();$Record.errors.GetAwaiter().GetResult()
    $Record.stdout.Flush();$Record.stderr.Flush();$Record.stdout.Dispose();$Record.stderr.Dispose()
    $Record.finished=$true
    if($ExpectedFailure){if($Record.process.ExitCode -eq 0){throw ('Expected rejection succeeded: '+$Record.label)}}
    elseif($Record.process.ExitCode -ne 0){throw ('Owned integration process failed: '+$Record.label+'; inspect private diagnostics.')}
}

function Invoke-IntegrationEntry([string]$Action,[string]$Label,[string]$Profile='Isolated',[string]$Config='',[string]$Username='',[string]$Password='',[switch]$ExpectedFailure){
    $arguments=@('-NoLogo','-NoProfile','-NonInteractive','-File',$entry,'-Profile',$Profile,'-Action',$Action,'-StateRoot',$context.root)
    if($Config){$arguments+=@('-ConfigPath',$Config)}
    if($Username){$arguments+=@('-Username',$Username)}
    if($Password){$arguments+=@('-PasswordFile',$Password)}
    $record=Start-IntegrationProcess 'pwsh' $arguments $Label
    Complete-IntegrationProcess $record -ExpectedFailure:$ExpectedFailure
}

function Invoke-IntegrationSQL([string]$SQL){
    Assert-LocalResources $context $state -RequireRunning
    $arguments=@(Get-LocalComposeArguments $context $state)+@('exec','-T','mysql','sh','-c','MYSQL_PWD="$(cat /run/secrets/mysql-root-password)" exec mysql --batch --skip-column-names --default-character-set=utf8mb4 -h 127.0.0.1 -u root game_realtime_local')
    $start=[Diagnostics.ProcessStartInfo]::new((Get-Command docker -CommandType Application).Source)
    foreach($argument in $arguments){$start.ArgumentList.Add($argument)}
    $start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardInput=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    $process=[Diagnostics.Process]::Start($start)
    try{
        $output=$process.StandardOutput.ReadToEndAsync();$errors=$process.StandardError.ReadToEndAsync()
        $process.StandardInput.WriteLine($SQL);$process.StandardInput.Close()
        if(!$process.WaitForExit(30000)){$process.Kill($true);throw 'Owned SQL observation timed out.'}
        $text=$output.GetAwaiter().GetResult();[void]$errors.GetAwaiter().GetResult()
        if($process.ExitCode -ne 0){throw 'Owned SQL observation failed; no dependency diagnostics were published.'}
        $text.TrimEnd("`r","`n")
    }finally{$process.Dispose()}
}

function Get-IntegrationSnapshot {
    # No credential, password hash, player body or SQL text is published. The
    # snapshots bound this Check window; they do not claim all server reads are
    # free of internal storage bookkeeping.
    Invoke-IntegrationSQL @'
SET SESSION group_concat_max_len=1048576;
SELECT 'columns',SHA2(GROUP_CONCAT(CONCAT_WS('/',table_name,column_name,column_type,is_nullable,COALESCE(column_default,'<null>')) ORDER BY table_name,ordinal_position SEPARATOR '|'),256) FROM information_schema.columns WHERE table_schema=DATABASE();
SELECT 'indexes',SHA2(GROUP_CONCAT(CONCAT_WS('/',table_name,index_name,seq_in_index,column_name,non_unique) ORDER BY table_name,index_name,seq_in_index SEPARATOR '|'),256) FROM information_schema.statistics WHERE table_schema=DATABASE();
SELECT 'migrations',COUNT(*),SHA2(GROUP_CONCAT(CONCAT_WS('/',migration_id,checksum,status) ORDER BY migration_id SEPARATOR '|'),256) FROM schema_migrations;
SELECT 'counts',(SELECT COUNT(*) FROM admins),(SELECT COUNT(*) FROM players),(SELECT COUNT(*) FROM player_assets),(SELECT COUNT(*) FROM asset_ledger),(SELECT COUNT(*) FROM pve_runs),(SELECT COUNT(*) FROM pve_reward_grants),(SELECT COUNT(*) FROM pve_outbox_records);
'@
}

function Get-IntegrationFailureCategories {
    $categories=[Collections.Generic.HashSet[string]]::new()
    $patterns=[ordered]@{
        permission_denied='(?i)permission denied|access is denied|unauthorizedaccessexception'
        compile_failed='(?i)compile failed|build failed|undefined:|cannot use |syntax error'
        ENV_RESOURCE_BLOCKED='ENV_RESOURCE_BLOCKED'
        storage_check_failed='(?i)local storage check failed|owned SQL observation failed|service readiness timed out|did not become ready'
        admin_creation_failed='(?i)administrator creation failed'
        ownership_mismatch='(?i)ownership does not match|identity mismatch|unrecognized state|foreign container'
    }
    foreach($path in Get-ChildItem -LiteralPath $evidence -File -Filter '*.log'){
        $reader=[IO.StreamReader]::new($path.FullName)
        try{
            $buffer=[char[]]::new(262144)
            $count=$reader.ReadBlock($buffer,0,$buffer.Length)
            $text=[string]::new($buffer,0,$count)
            foreach($entry in $patterns.GetEnumerator()){if($text -match $entry.Value){[void]$categories.Add($entry.Key)}}
        }finally{$reader.Dispose()}
    }
    if(!$success -and !$categories.Count){[void]$categories.Add('uncategorized_failure')}
    @($categories|Sort-Object)
}

function Get-IntegrationOwnedContainerStates {
    if($null -eq $state -or $state.owner -notmatch '^[a-f0-9]{32}$' -or $state.compose_project -notmatch '^gm-local-[a-f0-9]{12}$'){return @()}
    try{
        $ids=@(Read-LocalDocker @('ps','--all','--filter',('label=com.docker.compose.project='+$state.compose_project),'--filter',('label=io.game-realtime-gm.local-owner='+$state.owner),'--format','{{.ID}}'))
        $observations=@()
        foreach($id in $ids){
            if($id -notmatch '^[a-f0-9]{12,64}$'){continue}
            $format='{"service":{{json (index .Config.Labels "com.docker.compose.service")}},"status":{{json .State.Status}},"exit_code":{{json .State.ExitCode}}}'
            $record=(Read-LocalDocker @('inspect','--type','container','--format',$format,$id))|ConvertFrom-Json
            if($record.service -in @('mysql','redis') -and $record.status -in @('created','running','paused','restarting','removing','exited','dead')){
                $observations+=@([pscustomobject]@{service=[string]$record.service;status=[string]$record.status;exit_code=[int]$record.exit_code})
            }
        }
        $observations
    }catch{return @()}
}

function Wait-IntegrationReady($Record,[string]$Address,[int]$TimeoutSeconds=90){
    $deadline=[DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while([DateTime]::UtcNow -lt $deadline){
        if($Record.process.HasExited){throw ('Owned service exited before readiness: '+$Record.label)}
        if(Test-LocalReadiness $Address){return}
        Start-Sleep -Milliseconds 200
    }
    throw ('Owned service readiness timed out: '+$Record.label)
}

try{
    foreach($name in @('GOMAXPROCS','GOFLAGS','GOTOOLCHAIN')){$saved[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
    $env:GOMAXPROCS='2';$env:GOFLAGS='-p=1';$env:GOTOOLCHAIN='local'
    Invoke-IntegrationEntry Prepare 'prepare-first'
    $state=Read-LocalInstallation $context
    $ownsInstallation=$true
    $installationHash=(Get-FileHash -LiteralPath (Join-Path $context.root 'installation.json') -Algorithm SHA256).Hash
    $privateHashes=@{}
    foreach($file in Get-ChildItem -LiteralPath $context.private -File){$privateHashes[$file.Name]=(Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash}
    Invoke-IntegrationEntry Prepare 'prepare-repeat'
    Assert-Integration ((Get-FileHash -LiteralPath (Join-Path $context.root 'installation.json') -Algorithm SHA256).Hash -eq $installationHash) 'repeat_prepare_keeps_installation_identity'
    foreach($file in Get-ChildItem -LiteralPath $context.private -File){Assert-Integration ((Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash -eq $privateHashes[$file.Name]) ('repeat_prepare_keeps_'+$file.Name)}
    Invoke-IntegrationEntry ServicesStart 'services-start'
    Invoke-IntegrationEntry Initialize 'initialize'
    $state=Read-LocalInstallation $context
    Assert-Integration ($state.initialization -eq 'INITIALIZED') 'empty_installation_initialized'
    Invoke-IntegrationEntry Check 'isolated-check'
    Assert-Integration ((Invoke-IntegrationSQL 'SELECT COUNT(*) FROM admins;') -eq '0') 'fixed_administrator_seed_not_imported'
    $adminDirectory=Join-Path $evidence 'operator'
    [void][IO.Directory]::CreateDirectory($adminDirectory);Set-LocalPrivatePermissions $adminDirectory
    $adminPassword=[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
    $adminPasswordPath=Join-Path $adminDirectory 'password.txt'
    [IO.File]::WriteAllText($adminPasswordPath,$adminPassword,[Text.UTF8Encoding]::new($false));Set-LocalSecretFilePermissions $adminPasswordPath
    $adminUsername='ci_operator'
    Invoke-IntegrationEntry CreateAdmin 'create-admin' -Username $adminUsername -Password $adminPasswordPath
    Assert-Integration ((Invoke-IntegrationSQL "SELECT COUNT(*) FROM admins WHERE username='ci_operator' AND role='operator';") -eq '1') 'explicit_operator_created'
    Invoke-IntegrationEntry CreateAdmin 'create-admin-duplicate' -Username $adminUsername -Password $adminPasswordPath -ExpectedFailure
    Assert-Integration ((Invoke-IntegrationSQL 'SELECT COUNT(*) FROM admins;') -eq '1') 'duplicate_admin_is_rejected_without_second_row'

    $existingDirectory=Join-Path $evidence 'existing'
    [void][IO.Directory]::CreateDirectory($existingDirectory);Set-LocalPrivatePermissions $existingDirectory
    $existingPath=Join-Path $existingDirectory 'configuration.json'
    Write-LocalJson $existingPath ([pscustomobject]@{schema_version=1;environment=[pscustomobject]@{
        APP_HOST='127.0.0.1';APP_PORT=[string]$state.http_port;GAMEPLAY_MODE='v2';DB_HOST='127.0.0.1';DB_PORT=[string]$state.mysql_port;DB_USER='gm_local';DB_NAME='game_realtime_local';DB_PASSWORD_FILE=(Join-Path $context.private 'db-password.txt');REDIS_ADDR=('127.0.0.1:'+$state.redis_port);REDIS_DB='0';REDIS_PASSWORD_FILE=(Join-Path $context.private 'redis-password.txt');JWT_SECRET_FILE=(Join-Path $context.private 'jwt-secret.txt')
    }})
    Set-LocalSecretFilePermissions $existingPath
    $originalReadOnly=Invoke-IntegrationSQL 'SELECT @@GLOBAL.read_only,@@GLOBAL.super_read_only;'
    Assert-Integration ($originalReadOnly -eq "0`t0") 'owned_empty_fixture_started_writable'
    $before=Get-IntegrationSnapshot
    [void](Invoke-IntegrationSQL 'SET GLOBAL super_read_only=ON;')
    $readOnlyWasSet=$true
    Assert-Integration ((Invoke-IntegrationSQL 'SELECT @@GLOBAL.read_only,@@GLOBAL.super_read_only;') -eq "1`t1") 'existing_check_guarded_by_owned_mysql_read_only'
    Invoke-IntegrationEntry Check 'existing-read-only-check' -Profile Existing -Config $existingPath
    $after=Get-IntegrationSnapshot
    Assert-Integration ($before -ceq $after) 'existing_check_preserves_schema_indexes_migration_ledger_and_observed_counts'
    $existingOutput=[IO.File]::ReadAllText((Join-Path $evidence 'existing-read-only-check.stdout.log'))
    Assert-Integration ($existingOutput.Contains('PASS_READ_ONLY_STORAGE_CHECK')) 'existing_check_reports_its_read_only_success'
    [void](Invoke-IntegrationSQL 'SET GLOBAL super_read_only=OFF; SET GLOBAL read_only=OFF;')
    $readOnlyWasSet=$false

    $server=Start-IntegrationProcess 'pwsh' @('-NoLogo','-NoProfile','-NonInteractive','-File',$entry,'-Action','Server','-StateRoot',$context.root) 'server'
    $base='http://127.0.0.1:'+$state.http_port
    Wait-IntegrationReady $server ($base+'/ready')
    Assert-Integration $true 'isolated_server_ready_with_real_storage'
    if($env:GM_BROWSER_SMOKE -eq '1'){
        foreach($path in @('frontend/gm-admin/node_modules','frontend/pve-demo/node_modules')){if(!(Test-Path -LiteralPath (Join-Path $project $path) -PathType Container)){throw 'Browser smoke requires frontend npm ci in this CI job.'}}
        $frontendEnvironment=@{PVE_BACKEND_URL=$base}
        $adminUI=Start-IntegrationProcess 'npm' @('run','dev','--','--host','127.0.0.1','--port','5173','--strictPort') 'gm-admin-ui' (Join-Path $project 'frontend/gm-admin') $frontendEnvironment
        $pveUI=Start-IntegrationProcess 'npm' @('run','dev','--','--host','127.0.0.1','--port','5174','--strictPort') 'pve-demo-ui' (Join-Path $project 'frontend/pve-demo') $frontendEnvironment
        Wait-IntegrationReady $adminUI 'http://127.0.0.1:5173/'
        Wait-IntegrationReady $pveUI 'http://127.0.0.1:5174/'
        $browserDirectory=Join-Path $evidence 'browser'
        [void][IO.Directory]::CreateDirectory($browserDirectory);Set-LocalPrivatePermissions $browserDirectory
        $browserEnvironment=@{GM_SMOKE_BASE_URL=$base;GM_SMOKE_ADMIN_URL='http://127.0.0.1:5173';GM_SMOKE_PVE_URL='http://127.0.0.1:5174';GM_SMOKE_ADMIN_USERNAME=$adminUsername;GM_SMOKE_ADMIN_PASSWORD=$adminPassword;GM_SMOKE_EVIDENCE_DIR=$browserDirectory}
        $browser=Start-IntegrationProcess 'node' @((Join-Path $PSScriptRoot 'browser-smoke.mjs')) 'browser' $project $browserEnvironment
        Complete-IntegrationProcess $browser -TimeoutSeconds 180
        $browserStatus='PASS'
        Assert-Integration $true 'browser_smoke_completed'
    }
    $success=$true
    $integrationStage='completed'
}finally{
    # Stop only process handles started by this script, including each handle's
    # own children. No process-name scan or broad Docker pruning is permitted.
    $failedStage=$integrationStage
    if(!$success){$containerStates=@(Get-IntegrationOwnedContainerStates)}
    try{
        foreach($record in $processes){
            if(!$record.finished){
                try{
                    if(!$record.process.HasExited){$record.process.Kill($true);[void]$record.process.WaitForExit(10000)}
                    $record.output.GetAwaiter().GetResult();$record.errors.GetAwaiter().GetResult()
                }finally{$record.stdout.Dispose();$record.stderr.Dispose();$record.finished=$true}
            }
        }
        if($ownsInstallation){
            if($readOnlyWasSet){[void](Invoke-IntegrationSQL 'SET GLOBAL super_read_only=OFF; SET GLOBAL read_only=OFF;')}
            # A damaged ownership record blocks cleanup rather than adopting a
            # same-named container or volume. This is disposable synthetic CI data.
            $state=Read-LocalInstallation $context
            Assert-LocalResources $context $state
            Invoke-IntegrationEntry ServicesStop 'services-stop'
            Assert-LocalResources $context $state
            Invoke-LocalCompose $context $state @('down','--volumes','--timeout','30')|Out-Null
            Assert-LocalResources $context $state -RequireAbsent
        }
    }catch{
        $cleanupFailed=$true
        $success=$false
    }finally{
        # ServicesStop adds one completed child too; dispose after it has run.
        foreach($record in $processes){$record.process.Dispose()}
        foreach($item in $saved.GetEnumerator()){[Environment]::SetEnvironmentVariable($item.Key,$item.Value,'Process')}
    }
    foreach($secretPath in @((Join-Path $context.root 'private'),(Join-Path $evidence 'operator'))){
        if($cleanupFailed){continue} # Retain original credentials if ownership cleanup could not finish.
        if(Test-Path -LiteralPath $secretPath){
            $resolved=[IO.Path]::GetFullPath($secretPath)
            if(!$resolved.StartsWith($evidence+[IO.Path]::DirectorySeparatorChar,[StringComparison]::Ordinal)){throw 'Integration secret cleanup boundary mismatch.'}
            Remove-Item -LiteralPath $resolved -Recurse -Force
        }
    }
    $summary=[pscustomobject]@{status=$(if($success){'PASS_LOCAL_LAUNCHER_REAL_STORAGE'}else{'FAIL_LOCAL_LAUNCHER_REAL_STORAGE'});stage=$failedStage;cleanup_failed=$cleanupFailed;failure_categories=@(Get-IntegrationFailureCategories);owned_container_states=$containerStates;checks=$checks.ToArray();browser=$browserStatus;existing_check_scope='guarded synthetic Check window: schema/index/migration-ledger fingerprints and observed application counts; not a claim of zero whole-server internal writes';credentials_published=$false}
    Write-LocalJson (Join-Path $evidence 'summary.json') $summary
    Write-LocalJson (Join-Path $parent 'gm-local-integration-summary.json') $summary
    $summary|ConvertTo-Json -Depth 5
    if($cleanupFailed){throw 'Owned integration cleanup could not be completed; inspect only the recorded project and private state.'}
}
