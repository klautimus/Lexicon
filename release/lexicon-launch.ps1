# Lexicon Auto-Launch Script
# Called by the Inno Setup installer to start Lexicon and open the browser.
# Parameters:
#   $appPath   - Full path to the lexicon.exe directory
#   $port      - Frontend port to poll and open

param(
    [Parameter(Mandatory=$true)]
    [string]$appPath,

    [Parameter(Mandatory=$true)]
    [string]$port
)

$ErrorActionPreference = "Stop"

# Start the Lexicon server
$exePath = Join-Path $appPath "lexicon.exe"
if (-not (Test-Path $exePath)) {
    Write-Warning "lexicon.exe not found at $exePath"
    exit 1
}

Start-Process $exePath -WorkingDirectory $appPath

# Poll until the server is ready
$url = "http://localhost:$port"
$maxRetries = 30
$retries = 0

while ($retries -lt $maxRetries) {
    try {
        $r = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
        if ($r.StatusCode -eq 200) {
            break
        }
    } catch {
        # Server not ready yet
    }
    Start-Sleep -Seconds 1
    $retries++
}

# Open the browser
Start-Process $url
