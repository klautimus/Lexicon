$ErrorActionPreference = "Stop"
Set-Location "C:\Users\kevin\CascadeProjects\lexicon\backend"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -o lexicon.exe ./cmd/server
Write-Output "BUILD OK"
