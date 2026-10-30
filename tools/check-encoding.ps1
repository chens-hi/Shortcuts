$ErrorActionPreference = 'Stop'
$root = 'D:\Workspaces\shortcuts'

# 文本文件白名单:脚本 + go + 常见文本资源;无扩展名只认 manifest
$textExts = @('.ps1', '.go', '.js', '.md', '.html', '.css', '.json', '.mod', '.sum', '.sh', '.bat', '.cmd', '.txt', '.yml', '.yaml', '.svg')
$skipDirs = @('.tmp', '.zcode', 'dist', 'node_modules', '.git', 'bin')

$strictUtf8 = [System.Text.UTF8Encoding]::new($false, $true)

$files = Get-ChildItem -Path $root -Recurse -File | Where-Object {
    $full = $_.FullName
    $inSkip = $skipDirs | Where-Object { $full -like "*\$_\*" }
    if ($inSkip) { return $false }
    if ($_.Extension) { $textExts -contains $_.Extension.ToLowerInvariant() }
    else { $_.Name -eq 'manifest' }
}

$violations = @()
$clean = 0
foreach ($f in $files) {
    $bytes = [System.IO.File]::ReadAllBytes($f.FullName)
    $rel = $f.FullName.Substring($root.Length + 1)

    $bom = 'none'
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) { $bom = 'UTF8-BOM' }
    elseif ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE) { $bom = 'UTF16-LE-BOM' }
    elseif ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFE -and $bytes[1] -eq 0xFF) { $bom = 'UTF16-BE-BOM' }

    $validUtf8 = $true
    try { [void]$strictUtf8.GetString($bytes) } catch { $validUtf8 = $false }

    if ($bom -ne 'none' -or -not $validUtf8) {
        $violations += [pscustomobject]@{ File = $rel; BOM = $bom; ValidUtf8 = $validUtf8 }
    } else {
        $clean++
    }
}

Write-Output "Scanned $($files.Count) files: $clean clean (UTF-8 no BOM), $($violations.Count) violations"
if ($violations.Count -gt 0) {
    Write-Output ''
    Write-Output 'Violations (expect UTF-8 without BOM):'
    $violations | Format-Table -AutoSize | Out-String -Width 200 | Write-Output
}
