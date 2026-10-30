param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [object[]]$Arguments
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot '..')

function Read-AppVersion {
    $manifestPath = Join-Path (Get-Location) "shortcuts\manifest"
    if (-not (Test-Path $manifestPath)) {
        throw "shortcuts\manifest not found"
    }
    foreach ($line in Get-Content $manifestPath -Encoding UTF8) {
        if ($line -match '^version=([^\r\n]+)$') {
            return $Matches[1].Trim()
        }
    }
    throw "version= missing in shortcuts\manifest"
}

$version = Read-AppVersion
# Each replacement must actually hit something, otherwise the script has been
# pointing at the wrong file (or the marker text drifted) and silently no-op'd.
# Counting hits turns that silent failure into a loud one.
$expectations = @()

$i18nPath = "shortcuts/app/ui/www/shortcuts-i18n.js"
$indexPath = "shortcuts/app/ui/www/index.html"
$readmePath = "README.md"

$utf8 = [System.Text.UTF8Encoding]::new($false)

$i18n = [System.IO.File]::ReadAllText((Resolve-Path $i18nPath).Path, $utf8)
$quote = '"'
$versionPattern = 'const SHORTCUTS_VERSION = "[^"]+";'
$i18nHits = [regex]::Matches($i18n, $versionPattern).Count
$i18n = [regex]::Replace($i18n, $versionPattern, "const SHORTCUTS_VERSION = $quote$version$quote;")
[System.IO.File]::WriteAllText((Resolve-Path $i18nPath).Path, $i18n, $utf8)
$expectations += ,@('SHORTCUTS_VERSION in shortcuts-i18n.js', $i18nHits)

$index = [System.IO.File]::ReadAllText((Resolve-Path $indexPath).Path, $utf8)
$badgePattern = '(<span class="badge" id="versionBadge">)v[^<]+(</span>)'
$badgeHits = [regex]::Matches($index, $badgePattern).Count
$index = [regex]::Replace($index, $badgePattern, "`$1v$version`$2")
[System.IO.File]::WriteAllText((Resolve-Path $indexPath).Path, $index, $utf8)
$expectations += ,@('versionBadge span in index.html', $badgeHits)

$readmeLines = [System.IO.File]::ReadAllLines((Resolve-Path $readmePath).Path, $utf8)
$prefix = [string][char]0x5F53 + [string][char]0x524D + [string][char]0x6D4B + [string][char]0x8BD5 + [string][char]0x5305 + [string][char]0x7248 + [string][char]0x672C + [string][char]0xFF1A
$readmeHits = 0
for ($i = 0; $i -lt $readmeLines.Count; $i++) {
    if ($readmeLines[$i].StartsWith($prefix)) {
        $readmeLines[$i] = $prefix + '`' + $version + '`' + [string][char]0x3002
        $readmeHits++
    }
}
[System.IO.File]::WriteAllLines((Resolve-Path $readmePath).Path, $readmeLines, $utf8)
$expectations += ,@('version line in README.md', $readmeHits)

$missed = @($expectations | Where-Object { $_[1] -le 0 })
foreach ($one in $missed) {
    Write-Warning "version sync marker not found: $($one[0])"
}
if ($missed.Count -gt 0) {
    throw "version sync failed: $($missed.Count) marker(s) not found, bump the version in shortcuts\manifest first"
}

Write-Output "shortcuts version synced: $version"

if ($Arguments.Count -gt 0) {
    & @($Arguments)
}