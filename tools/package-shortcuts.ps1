param(
    [string]$FnpackPath = '',
    [string]$SourceDir = 'shortcuts',
    [string]$OutputDir = 'dist',
    [string]$WorkDir = ''
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot '..')

# Resolve fnpack
$fnpackFilter = if ($env:OS -eq 'Windows_NT' -or $IsWindows) { 'fnpack-*-windows-amd64.exe' } else { 'fnpack-*-linux-amd64' }
if (-not $FnpackPath) {
    $toolsRoot = Join-Path (Join-Path $PSScriptRoot '..') 'tools'
    $roots = @($PSScriptRoot, $toolsRoot)
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
    throw "fnpack not found: drop $fnpackFilter into tools\, or add fnpack to PATH"
}

# Resolve Go
$goExe = $null
$onPath = Get-Command go -ErrorAction SilentlyContinue
if ($onPath) { $goExe = $onPath.Source }
else {
    foreach ($candidate in @('C:\Program Files\Go\bin\go.exe', 'C:\Program Files (x86)\Go\bin\go.exe')) {
        if (Test-Path $candidate) { $goExe = $candidate; break }
    }
}
if (-not $goExe) { throw 'go not found' }

$manifestPath = Join-Path (Get-Location) "$SourceDir\manifest"
$manifestOriginal = [System.IO.File]::ReadAllText($manifestPath)
$versionLine = [regex]::Match($manifestOriginal, '(?m)^version\s*=\s*(\S+)')
$version = $versionLine.Groups[1].Value
if (-not $version) { throw 'version= missing in manifest' }

$appBinDir = Join-Path $SourceDir 'app\bin'
if (-not (Test-Path $appBinDir)) {
    New-Item -ItemType Directory -Path $appBinDir | Out-Null
}

$distDir = Join-Path (Get-Location) $OutputDir
if (-not (Test-Path $distDir)) { New-Item -ItemType Directory -Path $distDir | Out-Null }

# Clean old non-version-matching packages
Get-ChildItem $distDir -Filter 'shortcuts-*.fpk' -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -notlike "shortcuts-$version-*" } |
    Remove-Item -Force

# --- Step 1: build dual-arch binaries ---
# Map Go GOARCH -> fnOS manifest platform name (also used as binary suffix).
# launcher maps `uname -m` back to these names at runtime.
$targets = @(
    @{ goarch = 'amd64'; fnos = 'x86' },
    @{ goarch = 'arm64'; fnos = 'arm' }
)

$env:GOOS = 'linux'; $env:CGO_ENABLED = '0'
foreach ($t in $targets) {
    Write-Output "--- Building linux-$($t.goarch) -> fnrt-$($t.fnos) ---"
    $env:GOARCH = $t.goarch
    & $goExe build -o (Join-Path $appBinDir "fnrt-$($t.fnos)") ./cmd
}

# --- Step 2: write arch-aware launcher script ---
# `uname -m` returns kernel-native names (x86_64, aarch64, loongarch64, riscv64, ...).
# fnOS manifest uses short names: x86 / arm / loongarch / risc-v / all.
# Map kernel name -> fnOS name, then exec "${0}-${fnos}".
$launcher = @"
#!/bin/bash
# shortcuts fnrt: arch-aware launcher
resolve_fnos_platform() {
    case "`$(uname -m)" in
        x86_64|i386|i686) echo x86 ;;
        aarch64|arm64|armv7l) echo arm ;;
        loongarch64) echo loongarch ;;
        riscv64) echo risc-v ;;
        *) echo unknown ;;
    esac
}
fnos=`$(resolve_fnos_platform)
if [ "`$fnos" != "unknown" ] && [ -x "`${0}-`$fnos" ]; then
    exec "`${0}-`$fnos" "`$@"
fi
echo "shortcuts: no binary for arch `$(uname -m)" >&2
exit 1
"@
[System.IO.File]::WriteAllText((Join-Path $appBinDir 'fnrt'), $launcher)

Write-Output "--- Packing platform=all (amd64 + arm64 merged) ---"

# --- Step 3: patch manifest platform -> all, run fnpack FROM source dir ---
$platformLine = [regex]::Match($manifestOriginal, '(?m)^platform\s*=.*$')
$patchedManifest = $manifestOriginal.Remove($platformLine.Index, $platformLine.Length).Insert(
    $platformLine.Index, "platform=all")

# fnpack uses .tmp/ as scratch relative to CWD, so build from the shortcuts/ dir itself.
# Keep CWD here repo-root; cd into SourceDir only for fnpack invocation.
$sourceAbs = (Resolve-Path $SourceDir).Path
$sourceFpk = Join-Path $sourceAbs 'shortcuts.fpk'
if (Test-Path $sourceFpk) { Remove-Item $sourceFpk -Force }

$oldTemp = $env:TEMP; $oldTmp = $env:TMP
$workTmp = if ($WorkDir) { $WorkDir } else { (Join-Path (Get-Location) '.tmp/fnpack') }
if (Test-Path $workTmp) { Remove-Item $workTmp -Recurse -Force }
New-Item -ItemType Directory -Path $workTmp | Out-Null
$env:TEMP = $workTmp; $env:TMP = $workTmp

try {
    [System.IO.File]::WriteAllText($manifestPath, $patchedManifest)
    Push-Location $sourceAbs
    try {
        & $FnpackPath build 2>&1
    } finally {
        Pop-Location
        [System.IO.File]::WriteAllText($manifestPath, $manifestOriginal)
    }
} finally {
    $env:TEMP = $oldTemp; $env:TMP = $oldTmp
    $env:GOARCH = ''
}

if (-not (Test-Path $sourceFpk)) {
    throw "packaged file not found at $sourceFpk"
}

$versionedPackage = Join-Path $distDir "shortcuts-$version.fpk"
Move-Item $sourceFpk $versionedPackage -Force

# Clean scratch
if (Test-Path $workTmp) { Remove-Item $workTmp -Recurse -Force }

Write-Output "Package output: $versionedPackage"
Write-Output "  (amd64 + arm64 merged, platform=all)"
