param([string]$ToolsRoot)
$ErrorActionPreference = 'Stop'
if ($ToolsRoot) { . (Join-Path $ToolsRoot 'activate.ps1') }
$moduleRoot = Split-Path -Parent $PSScriptRoot
Push-Location $moduleRoot
try {
    if ((protoc --version) -ne 'libprotoc 36.2') { throw 'Protoc 36.2 is required.' }
    if ((protoc-gen-go --version) -notmatch 'v1.36.12$') { throw 'protoc-gen-go 1.36.12 is required.' }
    if ((protoc-gen-go-grpc --version) -notmatch '1.6.2$') { throw 'protoc-gen-go-grpc 1.6.2 is required.' }
    protoc -I proto --go_out=gen/reporting/v1 --go_opt=paths=source_relative --go-grpc_out=gen/reporting/v1 --go-grpc_opt=paths=source_relative --csharp_out=gen/csharp --descriptor_set_out=proto/reporting.pb --include_imports reporting.proto
    if ($LASTEXITCODE -ne 0) { throw 'Contract generation failed.' }
    $sourceHash = (Get-FileHash proto/reporting.proto -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText((Join-Path $moduleRoot 'proto/source.sha256'),($sourceHash+"`n"),[Text.UTF8Encoding]::new($false))
    go test ./internal/experiment -run '^TestContractBaselineAndUnknownFields$'
    if ($LASTEXITCODE -ne 0) { throw 'Contract compatibility failed.' }
} finally { Pop-Location }
