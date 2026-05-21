param(
    [string]$Server = $env:DUNE_SERVER,
    [string]$User = $(if ($env:DUNE_SSH_USER) { $env:DUNE_SSH_USER } else { "ubuntu" }),
    [string]$RemotePath = "/tmp/market-bot-linux",
    [string]$RemoteItemData = "/tmp/item-data.json",
    [string]$DbUser = "dune",
    [string]$DbPass = "",
    [string]$DbName = "dune",
    [int]$DbPort = 15432,
    [string]$CacheDb = "/tmp/market-bot-cache.db",
    [string]$BuyInterval = "5m",
    [string]$ListInterval = "30m",
    [double]$BuyThreshold = 1.05,
    [int]$MaxBuys = 50,
    [switch]$DryRun,
    [string]$StatusInterval = "10s",
    [switch]$NoUI
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Server)) {
    throw "Server required. Pass -Server <host> or set DUNE_SERVER."
}

$Root = Split-Path -Parent $PSScriptRoot
$RepoRoot = Split-Path -Parent $Root
$Binary = Join-Path $Root "market-bot-linux"
$ItemData = Join-Path $RepoRoot "dune-admin\item-data.json"
$Remote = "$User@$Server"

Write-Host "Building Linux market bot..." -ForegroundColor Cyan
Push-Location $Root
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags="-s -w" -o "market-bot-linux" .
}
finally {
    Pop-Location
    Remove-Item Env:\CGO_ENABLED, Env:\GOOS, Env:\GOARCH -ErrorAction SilentlyContinue
}

if (!(Test-Path $Binary)) {
    throw "Build failed: $Binary was not created"
}
if (!(Test-Path $ItemData)) {
    throw "Missing item data: $ItemData"
}

Write-Host "Uploading binary to ${Remote}:$RemotePath ..." -ForegroundColor Cyan
scp $Binary "${Remote}:$RemotePath"

Write-Host "Uploading item data to ${Remote}:$RemoteItemData ..." -ForegroundColor Cyan
scp $ItemData "${Remote}:$RemoteItemData"

Write-Host "Starting market bot on server (live update mode, not report mode)..." -ForegroundColor Cyan
$dryRunArg = if ($DryRun) { " -dryrun" } else { "" }
$uiArg = if ($NoUI) { " -ui=false" } else { " -ui=true" }
$remoteCommand = "chmod +x $RemotePath && TERM=xterm-256color $RemotePath -mode amp -report=false$dryRunArg$uiArg -dbuser '$DbUser' -dbpass '$DbPass' -dbname '$DbName' -dbport $DbPort -itemdata '$RemoteItemData' -cachedb '$CacheDb' -buyinterval '$BuyInterval' -listinterval '$ListInterval' -buythreshold $BuyThreshold -maxbuys $MaxBuys -statusinterval '$StatusInterval'"
ssh -t $Remote $remoteCommand
