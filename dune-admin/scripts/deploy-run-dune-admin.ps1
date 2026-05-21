param(
    [string]$Server = $env:DUNE_SERVER,
    [string]$User = $(if ($env:DUNE_SSH_USER) { $env:DUNE_SSH_USER } else { "ubuntu" }),
    [string]$RemotePath = "/tmp/dune-admin-amp",
    [string]$DbUser = "dune",
    [string]$DbPass = "",
    [string]$DbName = "dune",
    [int]$DbPort = 15432,
    [string]$Schema = "dune"
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Server)) {
    throw "Server required. Pass -Server <host> or set DUNE_SERVER."
}

$Root = Split-Path -Parent $PSScriptRoot
$Binary = Join-Path $Root "dune-admin-amp"
$Remote = "$User@$Server"

Write-Host "Building Linux binary..." -ForegroundColor Cyan
Push-Location $Root
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o "dune-admin-amp" .
}
finally {
    Pop-Location
}

if (!(Test-Path $Binary)) {
    throw "Build failed: $Binary was not created"
}

Write-Host "Uploading to ${Remote}:$RemotePath ..." -ForegroundColor Cyan
scp $Binary "${Remote}:$RemotePath"

Write-Host "Starting dune-admin on server..." -ForegroundColor Cyan
$remoteCommand = "chmod +x $RemotePath && TERM=xterm-256color $RemotePath -mode amp -dbuser '$DbUser' -dbpass '$DbPass' -dbname '$DbName' -dbport $DbPort -schema '$Schema'"
ssh -t $Remote $remoteCommand
