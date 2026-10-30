param(
    # 可选：显式指定新版本（MAJOR.MINOR.PATCH）。缺省时只递增最低位。
    [string]$Version = ''
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot '..')

$utf8 = [System.Text.UTF8Encoding]::new($false)
$manifestPath = Join-Path (Get-Location) 'shortcuts\manifest'
if (-not (Test-Path $manifestPath)) {
    throw "shortcuts\manifest not found"
}

# 只替换 version= 那一行，其余字节（含换行的 desc 中文）保持原样。
$manifestOriginal = [System.IO.File]::ReadAllText((Resolve-Path $manifestPath).Path, $utf8)
$versionMatch = [regex]::Match($manifestOriginal, '(?m)^version\s*=\s*([^\r\n]+)\s*$')
if (-not $versionMatch.Success) {
    throw "version= line not found in shortcuts\manifest"
}
$oldVersion = $versionMatch.Groups[1].Value.Trim()

$versionPattern = '^(\d+)\.(\d+)\.(\d+)$'
$oldParts = [regex]::Match($oldVersion, $versionPattern)
if (-not $oldParts.Success) {
    throw "unsupported version format in manifest: '$oldVersion' (expected MAJOR.MINOR.PATCH)"
}

$newVersion = ''
if ($Version) {
    if (-not ([regex]::Match($Version, $versionPattern)).Success) {
        throw "invalid -Version: '$Version' (expected MAJOR.MINOR.PATCH)"
    }
    if ($Version -eq $oldVersion) {
        throw "-Version '$Version' equals current manifest version; nothing to bump"
    }
    $newVersion = $Version
} else {
    $major = [int]$oldParts.Groups[1].Value
    $minor = [int]$oldParts.Groups[2].Value
    $patch = [int]$oldParts.Groups[3].Value
    $newVersion = "$major.$minor.$($patch + 1)"
}

$patchedManifest = $manifestOriginal.Remove($versionMatch.Index, $versionMatch.Length).Insert(
    $versionMatch.Index, "version=$newVersion")
[System.IO.File]::WriteAllText((Resolve-Path $manifestPath).Path, $patchedManifest, $utf8)

Write-Output "manifest version: $oldVersion -> $newVersion"

# 同步 UI 徽标、i18n 常量与 README 版本行；失败会 throw，这里不再吞错。
& (Join-Path $PSScriptRoot 'sync-version.ps1')
