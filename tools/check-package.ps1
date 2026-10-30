param(
    [Parameter(Mandatory = $true)]
    [string]$PackagePath
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot '..')

$dest = Join-Path (Get-Location) '.tmp/shortcuts.fpk.check'
$appDest = Join-Path $dest 'app.extracted'
if (Test-Path $dest) {
    Remove-Item $dest -Recurse -Force
}
New-Item -ItemType Directory -Path $dest | Out-Null

try {
    # tar 在 Windows（System32 自带）与 Linux 均可用，直接按名字调用保持跨平台。
    # 存档名一律用相对路径：GNU tar 会把带盘符的绝对路径当成 host:path
    # （"Cannot connect to D:"）。目录参数转成正斜杠，Git 的 MSYS tar 才能
    # 正确识别 Windows 路径（反斜杠会被转义成不可打开的名字）。
    $packageDir = Split-Path -Parent (Resolve-Path $PackagePath).Path
    $packageName = Split-Path -Leaf $PackagePath
    $destTar = $dest -replace '\\', '/'
    $appDestTar = $appDest -replace '\\', '/'
    Push-Location $packageDir
    try {
        & tar -xzf $packageName -C $destTar
    }
    finally {
        Pop-Location
    }
    New-Item -ItemType Directory -Path $appDest | Out-Null
    Push-Location $dest
    try {
        & tar -xzf 'app.tgz' -C $appDestTar
    }
    finally {
        Pop-Location
    }
    Write-Host '--- extracted package ---'

    $manifest = @(Get-Content (Join-Path $dest 'manifest') -Encoding UTF8 | Select-Object -First 20)
    Write-Host '--- manifest ---'
    foreach ($line in $manifest) { Write-Host $line }

    $indexPath = Join-Path $appDest 'ui/www/index.html'
    $i18nPath = Join-Path $appDest 'ui/www/shortcuts-i18n.js'
    $stylesPath = Join-Path $appDest 'ui/www/styles.css'
    $appJsPath = Join-Path $appDest 'ui/www/app.js'
    Write-Host '--- UI asset presence ---'
    foreach ($ap in @($indexPath, $stylesPath, $i18nPath, $appJsPath)) {
        if (Test-Path $ap) {
            Write-Host ('  OK  ' + (Split-Path $ap -Leaf))
        } else {
            Write-Warning ('MISSING ' + $ap)
        }
    }
    Write-Host '--- index.html version refs ---'
    Select-String -Path $indexPath -Pattern 'versionBadge|SHORTCUTS_VERSION|shortcuts ui version|<script|<link' | ForEach-Object { Write-Host $_.Line }
    Write-Host '--- styles.css head ---'
    Get-Content $stylesPath -TotalCount 3 -Encoding UTF8 | ForEach-Object { Write-Host $_ }
    Write-Host '--- shortcuts-i18n.js head ---'
    Get-Content $i18nPath -TotalCount 8 -Encoding UTF8 | ForEach-Object { Write-Host $_ }
    Write-Host '--- app.js head ---'
    Get-Content $appJsPath -TotalCount 3 -Encoding UTF8 | ForEach-Object { Write-Host $_ }
}
finally {
    if (Test-Path $dest) {
        Remove-Item $dest -Recurse -Force
    }
}