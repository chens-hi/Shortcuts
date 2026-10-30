param(
    [string]$Package = 'dist\shortcuts-latest-linux-amd64.fpk',
    [string]$WorkDir = '.tmp\pkgcheck'
)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot\..

if (-not (Test-Path $Package)) {
    throw "package not found: $Package"
}

if (-not (Test-Path $WorkDir)) {
    New-Item -ItemType Directory -Path $WorkDir | Out-Null
}

Get-ChildItem $WorkDir | Remove-Item -Recurse -Force

tar.exe -xzf $Package -C $WorkDir manifest app.tgz
tar.exe -xzf (Join-Path $WorkDir 'app.tgz') -C $WorkDir ui/www/index.html

Get-Content (Join-Path $WorkDir 'manifest') -Encoding UTF8
Select-String -Path (Join-Path $WorkDir 'ui\www\index.html') -Pattern 'icon-desc-row','description-field','webkit-resizer','versionBadge','SHORTCUTS_VERSION','div id="description"','icon-preview-wrap','icon-preview-overlay-btn','icon-preview-clear','icon-preview-restore','icon-preview-crop','iconClearButton','iconRestoreButton','iconCropButton','imageHash','currentHash === defaultHash','updateIconButtonVisibility','renderIconPreview','resetIconToDefault','restoreSavedIcon' | ForEach-Object { $_.Line.Trim() }

Write-Host '--- legacy/default-button references (should be empty) ---'
Select-String -Path (Join-Path $WorkDir 'ui\www\index.html') -Pattern 'iconDefaultButton','icon-actions','showCustomIconControls','hideClearIconControls','showDefaultIconControls','cropIcon','cleanupIcon','resetIconButton','restoreIconButton','resetIconTitle' | ForEach-Object { $_.Line.Trim() }
if ((Select-String -Path (Join-Path $WorkDir 'ui\www\index.html') -Pattern 'iconDefaultButton|icon-actions|showCustomIconControls|hideClearIconControls|showDefaultIconControls').Count -eq 0) {
    Write-Host 'no legacy icon-control references in packaged index.html'
}
