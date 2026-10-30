$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot '..')

$utf8 = New-Object System.Text.UTF8Encoding($false)
$workTmp = '.tmp/validate-ui'
if (-not (Test-Path $workTmp)) { New-Item -ItemType Directory -Path $workTmp | Out-Null }

$allOk = $true

function CheckFile($filePath, $label) {
    $result = node --check $filePath 2>&1
    if ($LASTEXITCODE -eq 0) { Write-Host "  ${label}: syntax OK" }
    else { Write-Host "  ${label}: SYNTAX FAIL: $result"; return $false }

    $content = [System.IO.File]::ReadAllText($filePath, $utf8)
    # Check for broken CSS selectors (backticks from PowerShell escaping)
    $badSelectors = [regex]::Matches($content, 'querySelector(?:All)?\([^)]*[``]')
    if ($badSelectors.Count -gt 0) {
        Write-Host "  ${label}: SELECTOR FAIL - backtick in querySelector"
        $allOk = $false
    }
    # Check for broken template-literal interpolation. PowerShell double-quoted
    # string escaping turns ${expr} into {expr}, so JS prints the braces literally
    # instead of evaluating the expression. node --check cannot catch this.
    # Only braces whose content looks like an expression (property access, call,
    # or logical operator) are flagged, so i18n tokens such as {name} are ignored.
    $brokenInterp = [regex]::Matches($content, '`[^`]*(?<!\$)\{[A-Za-z_$][A-Za-z0-9_$]*(?:\s*\.[A-Za-z0-9_$]+|\s*\([^)]*\)|\s*\|\||\s*&&|\s*\?\s*[^`}]*:)[^`]*`')
    if ($brokenInterp.Count -gt 0) {
        Write-Host "  ${label}: INTERPOLATION FAIL - ${brokenInterp.Count} template literal(s) with braces missing the dollar sign"
        $allOk = $false
    }
    # Check for unescaped Chinese quotes that break JS strings
    $lines = $content -split "`n"
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = $lines[$i]
        if ($line -match ':\s*"[^"]*"[^"]*"[^"]*",?\s*$') {
            $quotes = ($line.ToCharArray() | Where-Object { $_ -eq [char]0x22 }).Count
            if ($quotes -gt 4 -and $line -notmatch '\\') {
                Write-Host ("  ${label}: QUOTE WARN line " + ($i+1) + ": " + $line.Trim())
            }
        }
    }
    return $true
}

# Check standalone i18n
$i18nPath = 'shortcuts/app/ui/www/shortcuts-i18n.js'
if (Test-Path $i18nPath) {
    CheckFile $i18nPath 'i18n' | Out-Null
}

# Check standalone app.js
$appJsPath = 'shortcuts/app/ui/www/app.js'
if (Test-Path $appJsPath) {
    CheckFile $appJsPath 'app.js' | Out-Null
}

# Extract and check all inline <script> blocks from index.html
$htmlPath = 'shortcuts/app/ui/www/index.html'
if (Test-Path $htmlPath) {
    $html = [System.IO.File]::ReadAllText((Resolve-Path $htmlPath).Path, $utf8)
    $blocks = [regex]::Matches($html, '(?s)<script>(.*?)</script>')
    Write-Host ("  index.html: " + $blocks.Count + " script blocks")
    for ($i = 0; $i -lt $blocks.Count; $i++) {
        $code = $blocks[$i].Groups[1].Value.Trim()
        if ($code.Length -eq 0) { continue }
        $f = Join-Path $workTmp "block_$i.js"
        [System.IO.File]::WriteAllText($f, $code, $utf8)
        CheckFile $f ("block " + $i) | Out-Null
    }
}

# Cleanup
Remove-Item $workTmp -Recurse -Force -ErrorAction SilentlyContinue

if ($allOk) { Write-Host "=== UI VALIDATION PASS ==="; exit 0 }
else { Write-Host "=== UI VALIDATION FAIL ==="; exit 1 }