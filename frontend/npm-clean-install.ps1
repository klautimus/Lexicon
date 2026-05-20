$ErrorActionPreference = "Stop"
Set-Location "C:\Users\kevin\CascadeProjects\lexicon\frontend"

Write-Host "Removing node_modules..."
if (Test-Path "node_modules") {
    Remove-Item -Recurse -Force "node_modules"
}

Write-Host "Removing package-lock.json..."
if (Test-Path "package-lock.json") {
    Remove-Item -Force "package-lock.json"
}

Write-Host "Running npm install..."
npm install 2>&1

Write-Host "Done."
