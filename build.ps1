$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Push-Location $ScriptDir

# ── Windows ──
go build -o vcam.exe -ldflags="-s -w" .
if (-not $?) { Write-Host "[VCam] Windows build failed!" -ForegroundColor Red; exit 1 }
Write-Host "[VCam] Windows build: $ScriptDir\vcam.exe" -ForegroundColor Green

# ── Linux amd64 ──
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -o vcam-linux-amd64 -ldflags="-s -w" .
if (-not $?) { Write-Host "[VCam] Linux amd64 build failed!" -ForegroundColor Red; exit 1 }
Write-Host "[VCam] Linux amd64 build: $ScriptDir\vcam-linux-amd64" -ForegroundColor Green

# ── Linux arm64 ──
$env:GOOS="linux"; $env:GOARCH="arm64"
go build -o vcam-linux-arm64 -ldflags="-s -w" .
if (-not $?) { Write-Host "[VCam] Linux arm64 build failed!" -ForegroundColor Red; exit 1 }
Write-Host "[VCam] Linux arm64 build: $ScriptDir\vcam-linux-arm64" -ForegroundColor Green

# Reset env
$env:GOOS=""; $env:GOARCH=""

Pop-Location

Write-Host ""
Write-Host "[VCam] All builds done!" -ForegroundColor Cyan
Write-Host "  Release files:" -ForegroundColor Cyan
Write-Host "    vcam.exe          (Windows x64)" -ForegroundColor White
Write-Host "    vcam-linux-amd64  (Linux x86_64)" -ForegroundColor White
Write-Host "    vcam-linux-arm64  (Linux ARM64)" -ForegroundColor White
