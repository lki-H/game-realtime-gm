param(
    [Parameter(Mandatory=$true)][string]$PlayerPath,
    [Parameter(Mandatory=$true)][string]$WorkDirectory,
    [switch]$MixedParty
)

$ErrorActionPreference='Stop'
$repositoryRoot=Split-Path -Parent $PSScriptRoot
$server='http://127.0.0.1:8080'
if (!(Test-Path -LiteralPath $PlayerPath) -or !(Test-Path -LiteralPath $WorkDirectory)) { throw 'Existing player executable and work directory required' }
if ($env:DB_NAME -ne 'game_realtime_v2_test' -or $env:DB_PORT -ne '23306' -or $env:GAMEPLAY_MODE -ne 'v2') { throw 'Isolated V2 configuration required' }
if ($env:PVE_TEST_EVENTS_TOKEN.Length -lt 24) { throw 'Independent event credential required' }
$prefix='unity_'+[Guid]::NewGuid().ToString('N').Substring(0,12)
$password=[Guid]::NewGuid().ToString('N')
$players=@()
$ids=@()
$results=@()
$variables=@('PVE_DEMO_AUTOMATION','PVE_DEMO_SERVER','PVE_DEMO_USERNAME','PVE_DEMO_PASSWORD','PVE_DEMO_TASK','PVE_DEMO_RESULT','PVE_DEMO_PARTY_ROLE','PVE_DEMO_PEER','PVE_DEMO_INVITE_FILE')
$prior=@{}
foreach ($variable in $variables) { $prior[$variable]=[Environment]::GetEnvironmentVariable($variable,'Process') }
try {
    for ($index=0;$index-lt 4;$index++) {
        $body=@{username=$prefix+'_'+$index;password=$password;nickname='local Unity verification'} | ConvertTo-Json -Compress
        $registered=Invoke-RestMethod -Method Post -Uri ($server+'/api/register') -ContentType 'application/json' -Body $body
        if ($registered.code -ne 0) { throw 'Isolated account registration failed' }
        $ids+=$registered.data.id
    }
    $env:PVE_DEMO_AUTOMATION='1'
    $env:PVE_DEMO_SERVER=$server
    $env:PVE_DEMO_INVITE_FILE=Join-Path $WorkDirectory ($prefix+'-invite')
    $tasks=if($MixedParty){@('survivor','other_map','scout','')}else{@('hunter','technician','scout','')}
    for ($index=0;$index-lt 4;$index++) {
        $env:PVE_DEMO_USERNAME=$prefix+'_'+$index
        $env:PVE_DEMO_PASSWORD=$password
        $env:PVE_DEMO_TASK=$tasks[$index]
        $env:PVE_DEMO_RESULT=Join-Path $WorkDirectory ($prefix+'-result-'+$index+'.json')
        $env:PVE_DEMO_PARTY_ROLE=if($MixedParty -and $index-eq 0){'owner'}elseif($MixedParty -and $index-eq 1){'member'}else{''}
        $env:PVE_DEMO_PEER=[string]$ids[1]
        $results+=$env:PVE_DEMO_RESULT
        $players+=Start-Process -FilePath $PlayerPath -ArgumentList @('-batchmode','-nographics','-logFile',(Join-Path $WorkDirectory ($prefix+'-player-'+$index+'.log'))) -Environment @{PVE_TEST_EVENTS_TOKEN=$null;PVE_METRICS_TOKEN=$null;JWT_SECRET=$null;DB_PASSWORD=$null;PVE_TEST_PASSWORD=$null;MYSQL_ROOT_PASSWORD=$null;MYSQL_PASSWORD=$null;PVE_VERIFY_PASSWORD=$null;PVE_GRAFANA_PASSWORD=$null} -WindowStyle Hidden -PassThru
    }
    $env:PVE_DEMO_PASSWORD=$null
    $deadline=(Get-Date).AddMinutes(3)
    $submitted=$false
    while ((Get-Date)-lt $deadline) {
        $reports=@(foreach($path in $results){
            if(Test-Path -LiteralPath $path){
                $stream=$null
                $reader=$null
                try{
                    $stream=[IO.FileStream]::new($path,[IO.FileMode]::Open,[IO.FileAccess]::Read,([IO.FileShare]::ReadWrite-bor [IO.FileShare]::Delete))
                    $reader=[IO.StreamReader]::new($stream,[Text.Encoding]::UTF8)
                    $content=$reader.ReadToEnd()
                    if($content){ConvertFrom-Json -InputObject $content -ErrorAction SilentlyContinue}
                }catch [IO.IOException]{
                }finally{
                    if($reader){$reader.Dispose()}elseif($stream){$stream.Dispose()}
                }
            }
        })
        if (@($reports | Where-Object { $_.failure_type }).Count-gt 0) { $reports | Select-Object stage,failure_type,failure | Format-Table; throw 'Unity client verification failed' }
        if ($reports.Count-eq 4 -and !$submitted -and @($reports | Where-Object stage -ne 'assigned').Count-eq 0) {
            $runIDs=@($reports.run_id | Select-Object -Unique)
            if ($runIDs.Count-ne 1) { throw 'Unity players assigned to different runs' }
            Push-Location -LiteralPath (Join-Path $repositoryRoot 'backend')
            try {
                go run ./cmd/tools/pve_event_bot -run-id $runIDs[0] -players ($ids -join ',')
                if ($LASTEXITCODE-ne 0) { throw 'Trusted scenario submission failed' }
            } finally { Pop-Location }
            $submitted=$true
        }
        if ($reports.Count-eq 4 -and @($reports | Where-Object { !$_.success -or !$_.reconnected -or $_.result_status-ne 'settled' }).Count-eq 0) {
            foreach($process in $players){if(!$process.WaitForExit(10000)){throw 'Unity process failed to exit'};if($process.ExitCode-ne 0){throw 'Unity process exit failed'}}
            $reports | Select-Object player_id,run_id,party_id,success,reconnected,result_status | Format-Table -AutoSize
            Write-Output ('Unity verification passed; mixed_party='+[bool]$MixedParty+' prefix='+$prefix)
            return
        }
        Start-Sleep -Milliseconds 200
    }
    throw 'Unity verification exceeded three-minute deadline'
} finally {
    foreach($process in $players){if(!$process.HasExited){Stop-Process -Id $process.Id -ErrorAction SilentlyContinue}}
    foreach($variable in $variables){[Environment]::SetEnvironmentVariable($variable,$prior[$variable],'Process')}
}
