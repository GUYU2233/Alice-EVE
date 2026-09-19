param(
    [switch]$DesktopOnly,
    [switch]$MobileOnly
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$releaseDir = Join-Path $root 'releases'
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

if (-not $MobileOnly) {
    Push-Location (Join-Path $root 'desktop-app')
    try {
        wails build -clean -o Alice-EVE-Desktop.exe
        if ($LASTEXITCODE -ne 0) { throw "Desktop build failed with exit code $LASTEXITCODE" }
        Copy-Item -Force 'build/bin/Alice-EVE-Desktop.exe' (Join-Path $releaseDir 'Alice-EVE-Desktop.exe')
    } finally { Pop-Location }
}

if (-not $DesktopOnly) {
    Push-Location (Join-Path $root 'mobile-app')
    try {
        flutter build apk --release
        if ($LASTEXITCODE -ne 0) { throw "Mobile build failed with exit code $LASTEXITCODE" }
        Copy-Item -Force 'build/app/outputs/flutter-apk/app-release.apk' (Join-Path $releaseDir 'Alice-EVE-Mobile.apk')
    } finally { Pop-Location }
}

Get-ChildItem $releaseDir -File | Where-Object Name -In @('Alice-EVE-Desktop.exe','Alice-EVE-Mobile.apk') | Get-FileHash -Algorithm SHA256
