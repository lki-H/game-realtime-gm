# Shared implementation for the two explicit local launcher profiles. No helper
# evaluates configuration as PowerShell or prints credential values.
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'

function Assert-LocalExternalPath([string]$Path,[string]$Project){
    $value=[IO.Path]::GetFullPath($Path).TrimEnd('\','/')
    $source=[IO.Path]::GetFullPath($Project).TrimEnd('\','/')
    $comparison=[StringComparison]::OrdinalIgnoreCase
    foreach($separator in @('\','/')){
        if($value.Equals($source,$comparison) -or $value.StartsWith($source+$separator,$comparison) -or $source.StartsWith($value+$separator,$comparison)){throw 'Local state and credentials must be separate from the source tree.'}
    }
    $cursor=$value
    while($cursor){
        if(Test-Path -LiteralPath $cursor){
            if(((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'External state and credential paths cannot contain reparse points.'}
        }
        $parent=[IO.Path]::GetDirectoryName($cursor)
        if(!$parent -or $parent -eq $cursor){break};$cursor=$parent
    }
    $value
}

function Assert-LocalExternalFile([string]$Path,[string]$Project){
    $value=Assert-LocalExternalPath $Path $Project
    if(!(Test-Path -LiteralPath $value -PathType Leaf) -or (Get-Item -LiteralPath $value).Length -gt 65536){throw 'Expected a bounded external configuration or credential file.'}
    $value
}

function Get-LocalContext([string]$StateRoot=''){
    $project=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')).TrimEnd('\','/')
    $digest=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($project.ToLowerInvariant()))).ToLowerInvariant().Substring(0,12)
    if(!$StateRoot){$StateRoot=Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) ('GameRealtimeGM/local/'+$digest)}
    $root=Assert-LocalExternalPath $StateRoot $project
    [pscustomobject]@{project=$project;root=$root;private=(Join-Path $root 'private');identity=$digest;compose=(Join-Path $PSScriptRoot 'local-compose.yml')}
}

function Set-LocalPrivatePermissions([string]$Path){
    if($IsWindows){
        $info=[IO.DirectoryInfo]::new($Path)
        $acl=[Security.AccessControl.DirectorySecurity]::new()
        $acl.SetAccessRuleProtection($true,$false)
        foreach($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'))){
            $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))
        }
        [IO.FileSystemAclExtensions]::SetAccessControl($info,$acl)
    }else{
        & chmod 700 -- $Path
        if($LASTEXITCODE -ne 0){throw 'Could not protect the external state directory.'}
    }
}

function Assert-LocalPrivatePermissions([string]$Path){
    if($IsWindows){
        $acl=Get-Acl -LiteralPath $Path
        if(!$acl.AreAccessRulesProtected){throw 'The external private directory must disable inherited permissions.'}
        $allowed=@([Security.Principal.WindowsIdentity]::GetCurrent().User.Value,'S-1-5-18')
        foreach($rule in $acl.Access){
            if($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -notin $allowed){throw 'Private data is readable by an unexpected identity.'}
        }
    }else{
        if(([int][IO.File]::GetUnixFileMode($Path) -band 63) -ne 0){throw 'External private permissions must exclude group and other access.'}
    }
}

function Set-LocalSecretFilePermissions([string]$Path){
    if(!$IsWindows){[IO.File]::SetUnixFileMode($Path,([IO.UnixFileMode]::UserRead -bor [IO.UnixFileMode]::UserWrite))}
}

function Assert-LocalCredentialFile([string]$Path,[string]$Project){
    $value=Assert-LocalExternalFile $Path $Project
    Assert-LocalPrivatePermissions (Split-Path -Parent $value)
    if($IsWindows){
        $allowed=@([Security.Principal.WindowsIdentity]::GetCurrent().User.Value,'S-1-5-18')
        foreach($rule in (Get-Acl -LiteralPath $value).Access){
            if($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -notin $allowed){throw 'Credential file permissions allow an unexpected identity.'}
        }
    }elseif(([int][IO.File]::GetUnixFileMode($value) -band 63) -ne 0){throw 'Credential file permissions are too broad.'}
    $value
}

function Read-LocalSecret([string]$Path,[string]$Project){
    $value=Assert-LocalCredentialFile $Path $Project
    $text=[IO.File]::ReadAllText($value).TrimEnd("`r","`n")
    if(!$text -or $text.Length -gt 4096 -or $text -match '[\x00\r\n]'){throw 'Credential files must contain one bounded, nonempty value.'}
    $text
}

function Write-LocalJson([string]$Path,$Value){
    $temporary=$Path+'.'+[Guid]::NewGuid().ToString('N')+'.tmp'
    try{
        [IO.File]::WriteAllText($temporary,($Value|ConvertTo-Json -Depth 8)+"`n",[Text.UTF8Encoding]::new($false))
        [IO.File]::Move($temporary,$Path,$true)
    }finally{if(Test-Path -LiteralPath $temporary){Remove-Item -LiteralPath $temporary}}
}

function Read-LocalJson([string]$Path){
    if(!(Test-Path -LiteralPath $Path -PathType Leaf) -or (Get-Item -LiteralPath $Path).Length -gt 65536){throw 'Missing or oversized local configuration record.'}
    if(((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Configuration records cannot be reparse points.'}
    $value=[IO.File]::ReadAllText($Path)|ConvertFrom-Json -Depth 10
    if($value -isnot [pscustomobject]){throw 'Local configuration must be a JSON object.'}
    $value
}

function Assert-LocalKeys($Value,[string[]]$Allowed,[string[]]$Required){
    foreach($property in $Value.PSObject.Properties){
        if($property.Name -notin $Allowed){throw ('Unsupported configuration key: '+$property.Name)}
    }
    foreach($key in $Required){if($key -notin $Value.PSObject.Properties.Name){throw ('Missing explicit configuration key: '+$key)}}
}

function Get-LocalMemorySnapshot {
    if(!$IsWindows){return $null}
    try{
        $memory=Get-CimInstance Win32_OperatingSystem -ErrorAction Stop
        if($null -eq $memory.FreeVirtualMemory -or $null -eq $memory.FreePhysicalMemory){throw 'Missing memory counters.'}
        [pscustomobject]@{commit_bytes=([UInt64]$memory.FreeVirtualMemory*1KB);physical_bytes=([UInt64]$memory.FreePhysicalMemory*1KB)}
    }catch{throw 'ENV_RESOURCE_BLOCKED: system memory availability could not be checked; no new heavy process was started.'}
}

function Assert-LocalMemoryAdmission([ValidateSet('ServicesStart','Go')][string]$Workload){
    if(!$IsWindows){return}
    $memory=Get-LocalMemorySnapshot
    $commit=if($Workload -eq 'ServicesStart'){2688MB}else{1792MB}
    $physical=if($Workload -eq 'ServicesStart'){1536MB}else{1024MB}
    if($memory.commit_bytes -lt $commit -or $memory.physical_bytes -lt $physical){throw ('ENV_RESOURCE_BLOCKED: '+$Workload+' requires '+($commit/1MB)+' MiB available commit and '+($physical/1MB)+' MiB physical memory; nothing was started.')}
}

function Assert-LocalMemoryReserve {
    if(!$IsWindows){return}
    $memory=Get-LocalMemorySnapshot
    if($memory.commit_bytes -lt 1GB -or $memory.physical_bytes -lt 512MB){throw 'ENV_RESOURCE_BLOCKED: the unchanged 1 GiB commit / 512 MiB physical reserve was reached; this launcher-owned process was stopped.'}
}

function Start-LocalOwnedProcess([string]$Executable,[string[]]$Arguments){
    $command=Get-Command $Executable -CommandType Application -ErrorAction Stop|Select-Object -First 1
    $start=[Diagnostics.ProcessStartInfo]::new($command.Source)
    foreach($argument in $Arguments){$start.ArgumentList.Add($argument)}
    $start.WorkingDirectory=(Get-Location).Path
    $start.UseShellExecute=$false;$start.CreateNoWindow=$true
    $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    [Diagnostics.Process]::Start($start)
}

function Stop-LocalOwnedProcess($Process){
    if(!$Process.HasExited){$Process.Kill($true);[void]$Process.WaitForExit(5000)}
}

function Test-LocalReadiness([string]$Address){
    $handler=[Net.Http.HttpClientHandler]::new();$handler.UseProxy=$false
    $client=[Net.Http.HttpClient]::new($handler);$client.Timeout=[TimeSpan]::FromMilliseconds(500)
    try{
        $response=$client.GetAsync($Address).GetAwaiter().GetResult()
        try{return $response.StatusCode -eq [Net.HttpStatusCode]::OK}finally{$response.Dispose()}
    }catch{return $false}finally{$client.Dispose()}
}

function Wait-LocalOwnedProcess($Process,[string]$ReadyAddress=''){
    $ready=!$ReadyAddress;$deadline=[DateTime]::UtcNow.AddSeconds(30)
    $nextMemory=[DateTime]::MinValue
    $stdout=$Process.StandardOutput.ReadLineAsync();$stderr=$Process.StandardError.ReadLineAsync()
    try{
        while(!$Process.HasExited){
            if([DateTime]::UtcNow -ge $nextMemory){Assert-LocalMemoryReserve;$nextMemory=[DateTime]::UtcNow.AddSeconds(1)}
            foreach($stream in @('stdout','stderr')){
                for($lineCount=0;$lineCount -lt 128;$lineCount++){
                    $pending=if($stream -eq 'stdout'){$stdout}else{$stderr}
                    if($null -eq $pending -or !$pending.IsCompleted){break}
                    $line=$pending.GetAwaiter().GetResult()
                    if($null -ne $line){Write-Output $line}
                    if($stream -eq 'stdout'){$stdout=if($null -ne $line){$Process.StandardOutput.ReadLineAsync()}else{$null}}
                    else{$stderr=if($null -ne $line){$Process.StandardError.ReadLineAsync()}else{$null}}
                }
            }
            if(!$ready){
                if(Test-LocalReadiness $ReadyAddress){$ready=$true;Write-Host ('Service ready: '+$ReadyAddress)}
                elseif([DateTime]::UtcNow -ge $deadline){throw 'Owned service did not become ready within 30 seconds.'}
            }
            [void]$Process.WaitForExit(100)
        }
        # Drain outstanding reads only after the child has exited. Both pipes
        # have been consumed while it was running, preventing full-pipe deadlock.
        foreach($stream in @('stdout','stderr')){
            $pending=if($stream -eq 'stdout'){$stdout}else{$stderr}
            if($null -ne $pending){$line=$pending.GetAwaiter().GetResult();if($null -ne $line){Write-Output $line}}
            $reader=if($stream -eq 'stdout'){$Process.StandardOutput}else{$Process.StandardError}
            while($null -ne ($line=$reader.ReadLine())){Write-Output $line}
        }
        if(!$ready){throw 'Owned service exited before readiness was confirmed.'}
        if($Process.ExitCode -ne 0){throw ('Local owned process failed (exit '+$Process.ExitCode+').')}
    }finally{Stop-LocalOwnedProcess $Process;$Process.Dispose()}
}

function Invoke-LocalNative([string]$Executable,[string[]]$Arguments,[string]$ReadyAddress=''){
    if($Executable -eq 'go'){
        Assert-LocalMemoryAdmission Go
        $process=Start-LocalOwnedProcess $Executable $Arguments
        Wait-LocalOwnedProcess $process -ReadyAddress $ReadyAddress
        return
    }
    & $Executable @Arguments
    if($LASTEXITCODE -ne 0){throw ('Local command failed: '+$Executable+' (exit '+$LASTEXITCODE+').')}
}

function Read-LocalDocker([string[]]$Arguments){
    $result=@(& docker @Arguments 2>&1)
    if($LASTEXITCODE -ne 0){throw 'Docker resource identity could not be verified; no storage action was performed.'}
    $result
}

function Get-LocalComposeArguments($Context,$State){
    @('compose','--project-name',$State.compose_project,'--env-file',(Join-Path $Context.root 'compose.env'),'--file',$Context.compose)
}

function Invoke-LocalCompose($Context,$State,[string[]]$Arguments){
    $composeArguments=@(Get-LocalComposeArguments $Context $State)
    $values=@(& docker @composeArguments @Arguments 2>&1)
    if($LASTEXITCODE -ne 0){throw 'Owned local Compose action failed; original containers and credentials were retained.'}
    # Normal progress is not needed to prove ownership or readiness, and might
    # contain expanded values on a CLI error. Keep it out of public logs.
    Write-Output ('Owned Compose action completed: '+$Arguments[0])
}

function Assert-LocalResources($Context,$State,[switch]$RequireRunning,[switch]$RequireAbsent){
    $ids=@(Read-LocalDocker @('ps','--all','--filter',('label=com.docker.compose.project='+$State.compose_project),'--format','{{.ID}}'))
    $volumes=@(Read-LocalDocker @('volume','ls','--filter',('label=com.docker.compose.project='+$State.compose_project),'--format','{{.Name}}'))
    if($RequireAbsent){if($ids.Count -or $volumes.Count){throw 'Existing project resources require their original owned installation and credentials.'};return}
    if($volumes.Count -and !$ids.Count){throw 'Orphaned project volumes cannot be adopted automatically.'}
    $services=@{}
    foreach($id in $ids){
        if($id -notmatch '^[a-f0-9]{12,64}$'){throw 'Unexpected Docker container identity.'}
        $format='{"owner":{{json (index .Config.Labels "io.game-realtime-gm.local-owner")}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"running":{{json .State.Running}}}'
        $record=(Read-LocalDocker @('inspect','--type','container','--format',$format,$id))|ConvertFrom-Json
        if($record.owner -ne $State.owner -or $record.project -ne $State.compose_project -or $record.service -notin @('mysql','redis') -or $services.ContainsKey($record.service)){throw 'Container ownership does not match this installation.'}
        if($RequireRunning -and !$record.running){throw 'An owned storage service is not running.'}
        $services[$record.service]=$id
    }
    foreach($volume in $volumes){
        if($volume -notin @(($State.compose_project+'_mysql-data'),($State.compose_project+'_redis-data'))){throw 'Unexpected project volume name.'}
        $format='{"owner":{{json (index .Labels "io.game-realtime-gm.local-owner")}},"project":{{json (index .Labels "com.docker.compose.project")}}}'
        $record=(Read-LocalDocker @('volume','inspect','--format',$format,$volume))|ConvertFrom-Json
        if($record.owner -ne $State.owner -or $record.project -ne $State.compose_project){throw 'Volume ownership does not match this installation.'}
    }
    if($RequireRunning -and ($services.Count -ne 2 -or $volumes.Count -ne 2)){throw 'Start both owned storage roles before this action.'}
}

function Assert-LocalFreePorts([int[]]$Ports,[switch]$AllowOwned){
    $listeners=[Net.NetworkInformation.IPGlobalProperties]::GetIPGlobalProperties().GetActiveTcpListeners()
    foreach($port in $Ports){if($port -in @($listeners|ForEach-Object Port)){throw ('Port '+$port+' is already in use; no process was stopped and no port was changed.')}}
}

function Read-LocalInstallation($Context){
    [void](Assert-LocalExternalPath $Context.root $Context.project)
    Assert-LocalPrivatePermissions $Context.root
    Assert-LocalPrivatePermissions $Context.private
    $state=Read-LocalJson (Join-Path $Context.root 'installation.json')
    $keys=@('schema_version','source','compose_project','owner','http_port','mysql_port','redis_port','initialization','credential_hashes')
    Assert-LocalKeys $state $keys $keys
    if($state.schema_version -ne 1 -or $state.source -ne $Context.project -or $state.compose_project -ne ('gm-local-'+$Context.identity) -or $state.owner -notmatch '^[a-f0-9]{32}$' -or $state.http_port -ne 18080 -or $state.mysql_port -ne 33306 -or $state.redis_port -ne 36379 -or $state.initialization -notin @('NEW_EMPTY_INSTALLATION','INITIALIZATION_STARTED','INITIALIZED')){throw 'Invalid local installation identity; no resource was changed.'}
    $names=@('mysql-root-password.txt','db-password.txt','redis-password.txt','jwt-secret.txt','redis.conf')
    Assert-LocalKeys $state.credential_hashes $names $names
    $seen=@()
    foreach($name in $names){
        $path=Assert-LocalCredentialFile (Join-Path $Context.private $name) $Context.project
        if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $state.credential_hashes.$name){throw 'Owned credential file changed; refusing to start or reconfigure existing storage.'}
        if($name -ne 'redis.conf'){
            $secret=Read-LocalSecret $path $Context.project
            if($secret -notmatch '^[a-f0-9]{64}$' -or $secret -in $seen){throw 'Invalid or reused owned local credentials.'};$seen+=@($secret)
        }
    }
    $actual=@(Get-ChildItem -LiteralPath $Context.private -Force|ForEach-Object Name)
    if(@($actual|Where-Object{$_ -notin $names}).Count -or $actual.Count -ne $names.Count){throw 'Unknown or partial private installation contents.'}
    $envText=Get-LocalComposeEnvironmentText $Context $state
    [void](Assert-LocalExternalFile (Join-Path $Context.root 'compose.env') $Context.project)
    if(!(Test-Path -LiteralPath (Join-Path $Context.root 'compose.env')) -or [IO.File]::ReadAllText((Join-Path $Context.root 'compose.env')) -ne $envText){throw 'Local Compose configuration changed; refusing to change resource bindings.'}
    $state
}

function Get-LocalComposeEnvironmentText($Context,$State){
    $privatePath=$Context.private.Replace('\','/')
    if($privatePath -match '[\r\n\x00"$]'){throw 'State path cannot be represented safely in the Compose environment file.'}
    "GM_LOCAL_OWNER=$($State.owner)`nGM_LOCAL_PRIVATE=`"$privatePath`"`nGM_LOCAL_MYSQL_PORT=$($State.mysql_port)`nGM_LOCAL_REDIS_PORT=$($State.redis_port)`n"
}

function Initialize-LocalInstallation($Context){
    $statePath=Join-Path $Context.root 'installation.json'
    if(Test-Path -LiteralPath $statePath){$state=Read-LocalInstallation $Context;Assert-LocalResources $Context $state;Write-Output 'Existing owned local installation and credentials preserved.';return}
    if((Test-Path -LiteralPath $Context.root) -and @([IO.Directory]::EnumerateFileSystemEntries($Context.root)).Count){throw 'Partial or unrecognized state directory; no credentials were created or replaced.'}
    $state=[pscustomobject]@{schema_version=1;source=$Context.project;compose_project=('gm-local-'+$Context.identity);owner=[Guid]::NewGuid().ToString('N');http_port=18080;mysql_port=33306;redis_port=36379;initialization='NEW_EMPTY_INSTALLATION';credential_hashes=[pscustomobject]@{}}
    Assert-LocalResources $Context $state -RequireAbsent
    Assert-LocalFreePorts @(18080,33306,36379) -AllowOwned:$false
    [void][IO.Directory]::CreateDirectory($Context.root);Set-LocalPrivatePermissions $Context.root
    [void][IO.Directory]::CreateDirectory($Context.private);Set-LocalPrivatePermissions $Context.private
    foreach($name in @('mysql-root-password.txt','db-password.txt','redis-password.txt','jwt-secret.txt')){
        $secret=[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
        [IO.File]::WriteAllText((Join-Path $Context.private $name),$secret,[Text.UTF8Encoding]::new($false))
        Set-LocalSecretFilePermissions (Join-Path $Context.private $name)
    }
    $redisSecret=Read-LocalSecret (Join-Path $Context.private 'redis-password.txt') $Context.project
    [IO.File]::WriteAllText((Join-Path $Context.private 'redis.conf'),("bind 0.0.0.0`nprotected-mode yes`nport 6379`ndir /data`nappendonly yes`nrequirepass "+$redisSecret+"`n"),[Text.UTF8Encoding]::new($false))
    Set-LocalSecretFilePermissions (Join-Path $Context.private 'redis.conf')
    foreach($path in Get-ChildItem -LiteralPath $Context.private -File){$state.credential_hashes|Add-Member -NotePropertyName $path.Name -NotePropertyValue ((Get-FileHash -LiteralPath $path.FullName -Algorithm SHA256).Hash.ToLowerInvariant())}
    [IO.File]::WriteAllText((Join-Path $Context.root 'compose.env'),(Get-LocalComposeEnvironmentText $Context $state),[Text.UTF8Encoding]::new($false))
    Write-LocalJson $statePath $state
    Write-Output 'Created isolated local state and four independent credentials outside the repository; no containers or schema were created.'
}

function Get-LocalIsolatedEnvironment($Context,$State){
    @{
        APP_HOST='127.0.0.1';APP_PORT=[string]$State.http_port;GAMEPLAY_MODE='v2';GIN_MODE='release';GOMAXPROCS='2';GOFLAGS='-p=1';GOTOOLCHAIN='local'
        DB_HOST='127.0.0.1';DB_PORT=[string]$State.mysql_port;DB_NAME='game_realtime_local';DB_USER='gm_local';DB_PASSWORD=(Read-LocalSecret (Join-Path $Context.private 'db-password.txt') $Context.project)
        REDIS_ADDR=('127.0.0.1:'+$State.redis_port);REDIS_DB='0';REDIS_PASSWORD=(Read-LocalSecret (Join-Path $Context.private 'redis-password.txt') $Context.project)
        JWT_SECRET=(Read-LocalSecret (Join-Path $Context.private 'jwt-secret.txt') $Context.project)
        GM_LOCAL_OWNER=$State.owner;GM_LOCAL_PRIVATE=$Context.private.Replace('\','/');GM_LOCAL_MYSQL_PORT=[string]$State.mysql_port;GM_LOCAL_REDIS_PORT=[string]$State.redis_port
    }
}

function Read-LocalExistingEnvironment($Context,[string]$ConfigPath){
    $path=Assert-LocalExternalFile $ConfigPath $Context.project
    $configuration=Read-LocalJson $path
    Assert-LocalKeys $configuration @('schema_version','environment') @('schema_version','environment')
    if($configuration.schema_version -ne 1 -or $configuration.environment -isnot [pscustomobject]){throw 'Existing configuration requires schema_version=1 and an environment object.'}
    $required=@('APP_HOST','APP_PORT','GAMEPLAY_MODE','DB_HOST','DB_PORT','DB_USER','DB_NAME','DB_PASSWORD_FILE','REDIS_ADDR','REDIS_DB','REDIS_PASSWORD_FILE','JWT_SECRET_FILE')
    Assert-LocalKeys $configuration.environment $required $required
    $values=@{}
    foreach($property in $configuration.environment.PSObject.Properties){if($property.Value -isnot [string] -or $property.Value -match '[\r\n\x00]') {throw 'Existing environment values must be bounded single-line strings.'};if($property.Value.Length -gt 4096){throw 'Existing environment value is too long.'}}
    foreach($key in $required|Where-Object{!$_.EndsWith('_FILE')}){$values[$key]=$configuration.environment.$key}
    if($values.APP_HOST -notin @('127.0.0.1','::1') -or $values.DB_HOST -notin @('127.0.0.1','localhost') -or $values.REDIS_ADDR -notmatch '^(127\.0\.0\.1|localhost):([0-9]{1,5})$' -or $values.GAMEPLAY_MODE -notin @('v2','legacy') -or $values.DB_NAME -notmatch '^[a-zA-Z0-9_]{1,64}$' -or !$values.DB_USER){throw 'Existing requires explicit local listener and storage identities.'}
    foreach($port in @($values.APP_PORT,$values.DB_PORT,($values.REDIS_ADDR -split ':')[-1])){if($port -notmatch '^[0-9]{1,5}$' -or [int]$port -lt 1 -or [int]$port -gt 65535){throw 'Invalid Existing port.'}}
    if($values.DB_NAME -eq 'game_realtime_dc_demo' -or [int]$values.DB_PORT -eq 23316 -or $values.REDIS_ADDR -match ':26389$'){throw 'The Dungeon Contract installation is not an Existing profile target for this repository.'}
    if($values.REDIS_DB -notmatch '^([0-9]|1[0-5])$'){throw 'Redis database must be explicitly between 0 and 15.'}
    $values.DB_PASSWORD=Read-LocalSecret $configuration.environment.DB_PASSWORD_FILE $Context.project
    $values.JWT_SECRET=Read-LocalSecret $configuration.environment.JWT_SECRET_FILE $Context.project
    $values.REDIS_PASSWORD=if($configuration.environment.REDIS_PASSWORD_FILE){Read-LocalSecret $configuration.environment.REDIS_PASSWORD_FILE $Context.project}else{''}
    $values.GOMAXPROCS='2';$values.GIN_MODE='release';$values.GOFLAGS='-p=1';$values.GOTOOLCHAIN='local'
    $values
}

function Invoke-LocalSchema($Context,$State,[string]$Schema){
    $arguments=@(Get-LocalComposeArguments $Context $State)+@('exec','-T','mysql','sh','-c','MYSQL_PWD="$(cat /run/secrets/db-password)" exec mysql --default-character-set=utf8mb4 -h 127.0.0.1 -u gm_local game_realtime_local')
    # Byte redirection avoids PowerShell's text pipeline and preserves the SQL
    # source encoding. No user credential is included in the process arguments.
    $start=[Diagnostics.ProcessStartInfo]::new('docker')
    foreach($argument in $arguments){$start.ArgumentList.Add($argument)}
    $start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardInput=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
    $process=[Diagnostics.Process]::Start($start)
    try{
        $output=$process.StandardOutput.ReadToEndAsync();$errorOutput=$process.StandardError.ReadToEndAsync()
        $stream=[IO.File]::OpenRead($Schema)
        try{$stream.CopyTo($process.StandardInput.BaseStream)}finally{$stream.Dispose();$process.StandardInput.Close()}
        if(!$process.WaitForExit(60000)){$process.Kill();throw 'Owned schema initialization timed out; inspect the retained initialization state.'}
        [void]$output.GetAwaiter().GetResult();[void]$errorOutput.GetAwaiter().GetResult()
        if($process.ExitCode -ne 0){throw 'Owned schema initialization failed; no automatic retry or seed was performed.'}
    }finally{$process.Dispose()}
}
