# Local verification only: never contacts or modifies production services.
param([switch]$SkipFlutter)
$ErrorActionPreference='Stop'
$root=$PSScriptRoot
function Invoke-Check([string]$Directory,[string]$Program,[string[]]$Arguments) {
    Push-Location (Join-Path $root $Directory)
    try {
        Write-Host "=== $Directory : $Program $Arguments ==="
        & $Program @Arguments
        if($LASTEXITCODE -ne 0){throw "$Program failed (exit $LASTEXITCODE) in $Directory"}
    } finally {Pop-Location}
}
Invoke-Check 'relay-server' 'go' @('test','./...')
Invoke-Check 'desktop-app' 'go' @('test','./...')
Invoke-Check 'desktop-app/frontend' 'npm' @('run','build')
if(-not $SkipFlutter){
    Invoke-Check 'mobile-app' 'flutter' @('analyze')
    Invoke-Check 'mobile-app' 'flutter' @('test')
}
Write-Host 'Local checks passed. This does not verify device pairing, production persistence, or UI runtime behavior.'
