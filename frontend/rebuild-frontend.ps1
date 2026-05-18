# rebuild-frontend.ps1
# Clean rebuild of the Lexicon frontend (fixes rollup optional dep issues)

$ErrorActionPreference = "Stop"
$frontend = "C:\Users\kevin\CascadeProjects\lexicon\frontend"

Write-Host "=== Lexicon Frontend Rebuild ===" -ForegroundColor Cyan
Write-Host "Directory: $frontend"

# Step 1: Clean
Write-Host "`n[1/3] Cleaning node_modules and package-lock..." -ForegroundColor Yellow
Set-Location $frontend
if (Test-Path "package-lock.json") { Remove-Item "package-lock.json" -Force }
if (Test-Path "node_modules")      { Remove-Item "node_modules" -Recurse -Force }
Write-Host "  Cleaned." -ForegroundColor Green

# Step 2: Install
Write-Host "`n[2/3] Installing dependencies..." -ForegroundColor Yellow
& npm install
if ($LASTEXITCODE -ne 0) { throw "npm install failed with exit code $LASTEXITCODE" }
Write-Host "  Installed." -ForegroundColor Green

# Step 3: Build
Write-Host "`n[3/3] Building frontend..." -ForegroundColor Yellow
& npm run build
if ($LASTEXITCODE -ne 0) { throw "npm run build failed with exit code $LASTEXITCODE" }
Write-Host "`n=== Build succeeded! ===" -ForegroundColor Cyan
