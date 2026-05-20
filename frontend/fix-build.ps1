$ErrorActionPreference = "Continue"
Set-Location "C:\Users\kevin\CascadeProjects\lexicon\frontend"

Write-Host "=== Step 1: Kill any node/npm processes ==="
Get-Process | Where-Object { $_.ProcessName -match "node|npm" } | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

Write-Host "=== Step 2: Remove node_modules ==="
if (Test-Path "node_modules") {
    # Use robocopy to delete long paths
    $empty = "$env:TEMP\empty_dir"
    if (Test-Path $empty) { Remove-Item $empty -Recurse -Force }
    New-Item -ItemType Directory -Path $empty -Force | Out-Null
    robocopy $empty "node_modules" /MIR /NFL /NDL /NJH /NJS | Out-Null
    Remove-Item "node_modules" -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $empty -Recurse -Force -ErrorAction SilentlyContinue
}
Write-Host "node_modules removed"

Write-Host "=== Step 3: Remove lockfile ==="
Remove-Item "package-lock.json" -Force -ErrorAction SilentlyContinue

Write-Host "=== Step 4: Clean npm cache ==="
npm cache clean --force 2>&1 | Out-Null

Write-Host "=== Step 5: Fresh npm install ==="
npm install 2>&1

Write-Host "=== Step 6: Verify lucide-react ==="
$iconsDir = "node_modules/lucide-react/dist/esm/icons"
if (Test-Path $iconsDir) {
    $count = (Get-ChildItem $iconsDir -File).Count
    Write-Host "lucide-react icons: $count files"
} else {
    Write-Host "ERROR: lucide-react icons directory missing!"
}

Write-Host "=== Step 7: Build ==="
npm run build 2>&1

Write-Host "=== DONE ==="
