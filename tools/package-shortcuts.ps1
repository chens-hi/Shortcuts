param(
    [string]$FnpackPath = '',
    [string]$SourceDir = 'shortcuts',
    [string]$OutputDir = 'dist',
    [string[]]$Arch = @('amd64', 'arm64'),
    [string]$WorkDir = ''
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot '..')

# Resolve fnpack: explicit -FnpackPath, then a bundled copy, then PATH.
# fnpack is not part of this repo; in this checkout it ships in tools\, so
# search a couple of levels upward as well. Pick the binary matching the
# current OS so the same script works on Windows and Linux (CI) runners.
$fnpackFilter = if ($env:OS -eq 'Windows_NT' -or $IsWindows) { 'fnpack-*-windows-amd64.exe' } else { 'fnpack-*-linux-amd64' }
if (-not $FnpackPath) {
    $toolsRoot = Join-Path (Join-Path $PSScriptRoot '..') 'tools'
    $roots = @($PSScriptRoot, $toolsRoot, (Join-Path (Join-Path $toolsRoot '..') 'tools'))
    foreach ($root in $roots) {
        $bundled = Get-ChildItem -Path $root -Filter $fnpackFilter -ErrorAction SilentlyContinue |
            Select-Object -First 1 -ExpandProperty FullName
        if ($bundled) { $FnpackPath = $bundled; break }
    }
}
if (-not $FnpackPath) {
    $onPath = Get-Command fnpack -ErrorAction SilentlyContinue
    if ($onPath) { $FnpackPath = $onPath.Source }
}
if (-not $FnpackPath -or -not (Test-Path $FnpackPath)) {
    throw "fnpack not found: pass -FnpackPath, drop $fnpackFilter into tools/, or add fnpack to PATH"
}

# Resolve the Go toolchain: PATH first, then the standard install locations.
$goExe = $null
$onPath = Get-Command go -ErrorAction SilentlyContinue
if ($onPath) {
    $goExe = $onPath.Source
} else {
    foreach ($candidate in @('C:\Program Files\Go\bin\go.exe', 'C:\Program Files (x86)\Go\bin\go.exe')) {
        if (Test-Path $candidate) { $goExe = $candidate; break }
    }
    if (-not $goExe -and $env:GO_HOME) {
        $candidate = Join-Path $env:GO_HOME 'bin\go.exe'
        if (Test-Path $candidate) { $goExe = $candidate }
    }
}
if (-not $goExe) {
    throw 'go not found: install Go, set GO_HOME, or add Go\bin to PATH'
}

if (-not (Test-Path $SourceDir)) {
    throw "source directory not found: $SourceDir"
}

if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir | Out-Null
}

# Read version once
$manifestPath = Join-Path (Get-Location) 'shortcuts/manifest'
$version = foreach ($line in Get-Content $manifestPath -Encoding UTF8) {
    if ($line -match '^version=([^\r\n]+)$') {
        $Matches[1].Trim()
        break
    }
}

if (-not $version) {
    throw 'version= missing in shortcuts\manifest'
}

$distDir = Join-Path (Get-Location) $OutputDir

# Clean stale packages once before building.
# Keep all current-version packages; remove everything else.
Get-ChildItem $distDir -Filter 'shortcuts-*.fpk' -ErrorAction SilentlyContinue |
    Where-Object {
        $_.Name -notlike "shortcuts-$version-*"
    } |
    Remove-Item -Force

$appBinDir = Join-Path $SourceDir 'app\bin'
if (-not (Test-Path $appBinDir)) {
    New-Item -ItemType Directory -Path $appBinDir | Out-Null
}

foreach ($arch in $Arch) {
    $platform = "linux-$arch"
    # fnOS manifest 的 platform 只接受单个架构值：amd64 包声明 x86，arm64 包声明 arm。
    if ($arch -eq 'amd64') {
        $fnosPlatform = 'x86'
    }
    if ($arch -eq 'arm64') {
        $fnosPlatform = 'arm'
    }
    if (-not $fnosPlatform) {
        throw "unknown arch for fnOS platform mapping: $arch"
    }
    Write-Output "--- Building $platform ---"

    $goos = $env:GOOS
    $goarch = $env:GOARCH
    $cgoEnabled = $env:CGO_ENABLED
    $oldTemp = $env:TEMP
    $oldTmp = $env:TMP
    $workTmp = $null
    # 打包期间临时改写 manifest 的 platform 行，finally 里还原原文，
    # 即使中途失败也不会在仓库里留下脏 manifest。
    $manifestOriginal = [System.IO.File]::ReadAllText($manifestPath)
    try {
        $env:GOOS = 'linux'
        $env:GOARCH = $arch
        $env:CGO_ENABLED = '0'
        & $goExe build -o (Join-Path $appBinDir 'fnrt') ./cmd

        # fnpack needs a writable scratch dir: it creates a per-run temp dir and writes
        # the package into its own working directory. -WorkDir overrides the in-repo
        # default, which matters under the DSH sandbox where only the session temp
        # area is writable for native processes.
        if ($WorkDir) {
            $workTmp = $WorkDir
        } else {
            $workTmp = Join-Path (Get-Location) '.tmp/fnpack'
        }
        if (Test-Path $workTmp) {
            Remove-Item $workTmp -Recurse -Force -ErrorAction SilentlyContinue
        }
        New-Item -ItemType Directory -Path $workTmp -ErrorAction Stop | Out-Null

        $env:TEMP = $workTmp
        $env:TMP = $workTmp

        # 只替换 platform 那一行，其余字节（含换行符、中文）保持原样。
        $platformLine = [regex]::Match($manifestOriginal, '(?m)^platform\s*=.*$')
        if (-not $platformLine.Success) {
            throw "platform= line not found in $manifestPath"
        }
        $patchedManifest = $manifestOriginal.Remove($platformLine.Index, $platformLine.Length).Insert(
            $platformLine.Index, "platform=$fnosPlatform")
        [System.IO.File]::WriteAllText($manifestPath, $patchedManifest)

        # Run fnpack from the scratch dir rather than the repo root so the package
        # lands in a directory the native process can actually write to.
        Push-Location $workTmp
        try {
            & $FnpackPath build -d (Join-Path (Split-Path $PSScriptRoot -Parent) $SourceDir)
        }
        finally {
            Pop-Location
        }
    }
    finally {
        [System.IO.File]::WriteAllText($manifestPath, $manifestOriginal)
        if ($goos) { Set-Item Env:GOOS $goos } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
        if ($goarch) { Set-Item Env:GOARCH $goarch } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
        if ($cgoEnabled) { Set-Item Env:CGO_ENABLED $cgoEnabled } else { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
        $env:TEMP = $oldTemp
        $env:TMP = $oldTmp
        # Only clear fnpack's per-run scratch; the package itself has to survive to
        # the copy step below, so the scratch dir must stay.
        if ($workTmp) {
            Get-ChildItem $workTmp -Directory -Filter 'fnpack.*' -ErrorAction SilentlyContinue |
                Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    $packagedFile = Join-Path $workTmp 'shortcuts.fpk'
    if (-not (Test-Path $packagedFile)) {
        throw "packaged file not found: $packagedFile"
    }

    $versionedPackage = Join-Path $distDir "shortcuts-$version-$platform.fpk"

    Copy-Item $packagedFile $versionedPackage -Force

    # Clean root-level intermediate package
    Remove-Item $packagedFile -Force -ErrorAction SilentlyContinue

    # Clean fnpack work temp
    if (Test-Path $workTmp) {
        Remove-Item $workTmp -Recurse -Force
    }

    Write-Output "Package output: $versionedPackage"
}
