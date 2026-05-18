# Lexicon Release Build Pipeline
# Run this on the developer's Windows machine to produce LexiconSetup.exe
#
# Prerequisites:
#   - Node.js + npm (for frontend build)
#   - Go 1.22+ (for backend build)
#   - Inno Setup 6+ (for installer compilation)
#
# Usage:
#   cd release
#   powershell -ExecutionPolicy Bypass -File build.ps1

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$release = Join-Path $root "release"
$backend = Join-Path $root "backend"
$frontend = Join-Path $root "frontend"
$tools = Join-Path $root "tools"

function Step($msg) {
    Write-Host "`n==> $msg" -ForegroundColor Cyan
}

# 1. Build frontend
Step "Building frontend..."
Set-Location $frontend

# npm has a known bug (#4828) where optional platform-specific
# dependencies (@rollup/rollup-*) don't resolve on the first install
# and can get into a permanently broken state.  We check for both
# tsc (missing devDeps) and the CORRECT platform's rollup native
# module (a partial install often has Linux modules but not Windows).
# If either is missing the node_modules is corrupt — nuke and retry.
$tscExists = (Test-Path "node_modules\\.bin\\tsc.cmd") -or (Test-Path "node_modules\\.bin\\tsc")

# Determine which rollup native module we expect on this platform
if ($IsWindows -or ($env:OS -eq "Windows_NT")) {
    $rollupNative = "node_modules\\@rollup\\rollup-win32-x64-msvc"
} elseif ($IsMacOS) {
    $rollupNative = "node_modules\\@rollup\\rollup-darwin-arm64"
} else {
    $rollupNative = "node_modules\\@rollup\\rollup-linux-x64-gnu"
}
$rollupExists = Test-Path $rollupNative

if ((-not $tscExists) -or (-not $rollupExists)) {
    Write-Host "npm bug: node_modules is corrupt (missing devDeps or native modules). Reinstalling..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force node_modules -ErrorAction SilentlyContinue
    Remove-Item -Force package-lock.json -ErrorAction SilentlyContinue
}
if (-not (Test-Path "node_modules")) {
    npm install --include=dev
}
npm run build
if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }

# 2. Copy dist into backend embed directory
Step "Copying frontend/dist into backend embed directory..."
$embedDist = Join-Path $backend "cmd\server\dist"
if (Test-Path $embedDist) {
    Remove-Item -Recurse -Force $embedDist
}
Copy-Item -Recurse (Join-Path $frontend "dist") $embedDist

# 3. Build Go binary
Step "Building Go backend (single binary with embedded frontend)..."
Set-Location $backend
$binary = Join-Path $release "lexicon.exe"
go clean -cache
go build -ldflags "-s -w" -o $binary ./cmd/server
if ($LASTEXITCODE -ne 0) { throw "Go build failed" }
Write-Host "Binary: $binary ($((Get-Item $binary).Length / 1MB) MB)"

# 4. Gather tool binaries
Step "Gathering tool binaries..."
$releaseTools = Join-Path $release "tools"
if (-not (Test-Path $releaseTools)) {
    New-Item -ItemType Directory -Path $releaseTools | Out-Null
}

# Bundle spotiflac.exe if it exists
$spotiflacSrc = Join-Path $tools "spotiflac.exe"
if (Test-Path $spotiflacSrc) {
    Copy-Item $spotiflacSrc (Join-Path $releaseTools "spotiflac.exe")
    Write-Host "Bundled spotiflac.exe"
} else {
    Write-Warning "spotiflac.exe not found at $spotiflacSrc"
}

# Download yt-dlp.exe if missing
$ytDlpDest = Join-Path $releaseTools "yt-dlp.exe"
if (-not (Test-Path $ytDlpDest)) {
    Write-Host "Downloading yt-dlp.exe..."
    try {
        Invoke-WebRequest -Uri "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe" -OutFile $ytDlpDest -UseBasicParsing
        Write-Host "Downloaded yt-dlp.exe"
    } catch {
        Write-Warning "Failed to download yt-dlp.exe: $_"
    }
} else {
    Write-Host "Bundled yt-dlp.exe"
}

# Download spotdl.exe if missing
$spotdlDest = Join-Path $releaseTools "spotdl.exe"
if (-not (Test-Path $spotdlDest)) {
    Write-Host "Downloading spotdl.exe..."
    try {
        $releaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/spotDL/spotify-downloader/releases/latest" -UseBasicParsing
        $asset = $releaseInfo.assets | Where-Object { $_.name -like "*win32.exe" } | Select-Object -First 1
        if ($asset) {
            Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $spotdlDest -UseBasicParsing
            Write-Host "Downloaded spotdl.exe ($($asset.name))"
        } else {
            Write-Warning "No win32.exe asset found in latest SpotDL release"
        }
    } catch {
        Write-Warning "Failed to download spotdl.exe: $_"
    }
} else {
    Write-Host "Bundled spotdl.exe"
}

# Try to find and copy ngrok.exe (skip Windows App aliases which are 0-byte stubs)
$ngrokDest = Join-Path $releaseTools "ngrok.exe"
if (-not (Test-Path $ngrokDest)) {
    $ngrokSrc = (Get-Command ngrok -ErrorAction SilentlyContinue).Source
    if ($ngrokSrc) {
        $srcInfo = Get-Item $ngrokSrc
        if ($srcInfo.Length -gt 0) {
            Copy-Item $ngrokSrc $ngrokDest
            Write-Host "Bundled ngrok.exe"
        } else {
            Write-Warning "ngrok.exe is a Windows App alias (0-byte stub) - cannot bundle. Install from https://ngrok.com/download"
        }
    } else {
        Write-Warning "ngrok.exe not found in PATH. Install from https://ngrok.com/download"
    }
} else {
    Write-Host "Bundled ngrok.exe"
}

# Bundle poddl.exe (podcast downloader)
$poddlDest = Join-Path $releaseTools "poddl.exe"
if (-not (Test-Path $poddlDest)) {
    $poddlSrc = Join-Path $tools "poddl.exe"
    if (Test-Path $poddlSrc) {
        Copy-Item $poddlSrc $poddlDest
        Write-Host "Bundled poddl.exe"
    } else {
        Write-Warning "poddl.exe not found in tools/. Download from https://github.com/freshe/poddl"
    }
} else {
    Write-Host "Bundled poddl.exe"
}

# Check for ffmpeg.exe and ffprobe.exe (too large to auto-download reliably)
$ffmpegDest = Join-Path $releaseTools "ffmpeg.exe"
$ffprobeDest = Join-Path $releaseTools "ffprobe.exe"
if (-not (Test-Path $ffmpegDest)) {
    $ffmpegSrc = (Get-Command ffmpeg -ErrorAction SilentlyContinue).Source
    if ($ffmpegSrc) {
        Copy-Item $ffmpegSrc $ffmpegDest
        $ffprobeSrc = Join-Path (Split-Path $ffmpegSrc) "ffprobe.exe"
        if (Test-Path $ffprobeSrc) {
            Copy-Item $ffprobeSrc $ffprobeDest
        }
        Write-Host "Bundled ffmpeg.exe"
    } else {
        Write-Warning "ffmpeg.exe not found. Install from https://www.gyan.dev/ffmpeg/builds/ and place ffmpeg.exe in tools/"
    }
} else {
    Write-Host "Bundled ffmpeg.exe"
}
# Ensure ffprobe.exe is bundled alongside ffmpeg.exe
if ((Test-Path $ffmpegDest) -and (-not (Test-Path $ffprobeDest))) {
    $ffprobeSrc = Join-Path (Split-Path $ffmpegDest) "ffprobe.exe"
    if (Test-Path $ffprobeSrc) {
        Copy-Item $ffprobeSrc $ffprobeDest
        Write-Host "Bundled ffprobe.exe"
    } else {
        Write-Warning "ffprobe.exe not found next to ffmpeg.exe. Download from https://www.gyan.dev/ffmpeg/builds/"
    }
}

# 4b. Bundle icon files for installer
Step "Bundling icon files..."
$iconIco = Join-Path $release "lexicon.ico"
$iconSvg = Join-Path $release "icon.svg"
$icon192 = Join-Path $release "icon-192.png"
$icon512 = Join-Path $release "icon-512.png"
# Generate icons if not present
if (-not (Test-Path $iconIco)) {
    Write-Host "Generating icon files..."
    python3 (Join-Path $release "gen_icon.py")
}
if (Test-Path $iconIco) {
    Write-Host "Icon files ready: lexicon.ico, icon.svg, icon-192.png, icon-512.png"
} else {
    Write-Warning "Icon generation failed. Installer will use default icon."
}

# 5. Compile Inno Setup installer
Step "Compiling installer with Inno Setup..."
$iss = Join-Path $release "lexicon.iss"
$iscc = "C:\Program Files (x86)\Inno Setup 6\iscc.exe"
if (-not (Test-Path $iscc)) {
    $iscc = "iscc.exe"
}
& $iscc $iss
if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed" }

Step "Build complete!"
Write-Host "Output: $release\LexiconSetup.exe" -ForegroundColor Green
Set-Location $release
