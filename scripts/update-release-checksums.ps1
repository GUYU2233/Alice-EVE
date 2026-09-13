param([string]$ReleaseDirectory=(Join-Path $PSScriptRoot '..\releases'))
$ErrorActionPreference='Stop'
$directory=(Resolve-Path $ReleaseDirectory).Path
$artifacts=@(Get-ChildItem -LiteralPath $directory -File | Where-Object { $_.Extension -in '.exe','.apk','.zip' } | Sort-Object Name)
if($artifacts.Count -eq 0){throw 'No release artifacts found; checksum manifest unchanged.'}
$lines=@($artifacts | ForEach-Object { '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$_.Name })
$temporary=Join-Path $directory ('checksums-'+[guid]::NewGuid()+'.tmp')
try {
    $lines | Set-Content -LiteralPath $temporary -Encoding ascii
    Move-Item -LiteralPath $temporary -Destination (Join-Path $directory 'SHA256SUMS.txt') -Force
} finally { if(Test-Path -LiteralPath $temporary){Remove-Item -LiteralPath $temporary} }
Write-Host "Hashed $($artifacts.Count) current artifacts. Archive excluded."
