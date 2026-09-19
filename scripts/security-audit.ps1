[CmdletBinding()]
param(
    [string]$Repository = (Join-Path $PSScriptRoot '..'),
    [switch]$IncludeIgnored
)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path -LiteralPath $Repository).Path
Push-Location $repo
try {
    $gitArgs = @('ls-files')
    if ($IncludeIgnored) { $gitArgs += @('--cached', '--others', '--exclude-standard') }
    $files = @(git @gitArgs)
    if ($LASTEXITCODE -ne 0) { throw 'git ls-files failed; run this from a Git checkout.' }

    $findings = [System.Collections.Generic.List[string]]::new()
    $binaryExtensions = @('.apk','.appx','.bin','.dmg','.dll','.dylib','.exe','.ipa','.msi','.p12','.pfx','.so','.tar','.tgz','.war','.zip')
    $ignoredBinaryPaths = @('mobile-app/web/favicon.png','mobile-app/web/icons/')
    $publicIp = '\b(?!(?:10|127|169\.254|192\.168|192\.0\.2|198\.51\.100|203\.0\.113)\.)(?!(?:172\.(?:1[6-9]|2\d|3[01])))\d{1,3}(?:\.\d{1,3}){3}\b'
    $credential = '(?i)(?:password|passwd|secret|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret)\s*[:=]\s*["''`]?(?!CHANGE_ME|REPLACE_WITH|<redacted|<operator|example\.invalid|localhost|127\.0\.0\.1|\$|providers\.|null|releaseStorePassword|releaseKeyPassword|releaseStoreFile|releaseKeyAlias|release[A-Za-z]|=)[^\s"''`,;}]+'
    # Operator-specific identifiers belong in a private environment variable, never source control.
    # Example: ALICE_SECURITY_PRIVATE_PATTERN='(?i)(private-host-alias|private-domain\.invalid)'
    $knownLeak = [Environment]::GetEnvironmentVariable('ALICE_SECURITY_PRIVATE_PATTERN')
    $secretToken = '(?i)(?:gh[pousr]_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----|xox[baprs]-[0-9A-Za-z-]{20,})'

    foreach ($relative in $files) {
        $path = $relative -replace '\\','/'
        $full = Join-Path $repo $relative
        $extension = [IO.Path]::GetExtension($relative).ToLowerInvariant()
        if (($binaryExtensions -contains $extension) -and -not ($ignoredBinaryPaths | Where-Object { $path -like "$_*" })) {
            $findings.Add("BINARY $path")
            continue
        }
        try { $text = Get-Content -LiteralPath $full -Raw -ErrorAction Stop } catch { continue }
        $lineNumber = 0
        foreach ($line in ($text -split "`r?`n")) {
            $lineNumber++
            if ($knownLeak -and $path -ne 'scripts/security-audit.ps1' -and $line -match $knownLeak) { $findings.Add("INFRA ${path}:$lineNumber $($line.Trim())") }
            if ($line -match $publicIp) { $findings.Add("PUBLIC-IP ${path}:$lineNumber $($line.Trim())") }
            if ($line -match $credential) { $findings.Add("CREDENTIAL ${path}:$lineNumber $($line.Trim())") }
            if ($line -match $secretToken) { $findings.Add("SECRET ${path}:$lineNumber") }
        }
    }

    if ($findings.Count -gt 0) {
        Write-Host "Security audit failed: $($findings.Count) finding(s)" -ForegroundColor Red
        $findings | ForEach-Object { Write-Host " - $_" }
        exit 1
    }
    Write-Host "Security audit passed: $($files.Count) tracked file(s) checked; no known infrastructure leaks, credentials, or release binaries found." -ForegroundColor Green
} finally {
    Pop-Location
}
