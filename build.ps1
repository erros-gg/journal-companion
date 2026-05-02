param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

if (-not $Version) {
    $Version = (git describe --tags --always 2>$null)
    if (-not $Version) { $Version = "dev" }
}

$ldflags = "-H=windowsgui -s -w -X main.version=$Version"
$out     = "dist\journal-companion.exe"

Write-Host "Building $out (version: $Version)..."

New-Item -ItemType Directory -Force -Path dist | Out-Null

$env:GOOS        = "windows"
$env:GOARCH      = "amd64"
$env:CGO_ENABLED = "0"

go build -ldflags $ldflags -o $out .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed."
    exit 1
}

Write-Host "Done -> $out"
