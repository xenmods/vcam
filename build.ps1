$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Push-Location $ScriptDir
go build -o vcam.exe -ldflags="-s -w" .
if ($?) {
    Write-Host "[VCam] Build successful: $ScriptDir\vcam.exe" -ForegroundColor Green
} else {
    Write-Host "[VCam] Build failed!" -ForegroundColor Red
    exit 1
}
Pop-Location
