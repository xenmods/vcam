# VCam Bootstrap

Single Windows executable that downloads [go2rtc](https://github.com/AlexxIT/go2rtc) + ffmpeg,
generates a TLS certificate, and starts a go2rtc media server with screen capture
— ready to connect from your phone.

## Quick Start

```powershell
.\build.ps1
.\vcam.exe
```

Or build manually:

```powershell
go build -o vcam.exe -ldflags="-s -w" .
.\vcam.exe
```

## What it does

1. Downloads go2rtc + ffmpeg (auto-detects if already installed)
2. Generates a self-signed TLS cert with your LAN IP
3. Writes a go2rtc config that streams your desktop via gdigrab
4. Serves a mobile webapp over HTTPS on port **4321**
5. Optionally prompts to disable screen capture on low-end PCs

## Connect

Open `https://<YOUR_LAN_IP>:4321/` on your phone.

Use with the [VCam Minecraft mod](https://github.com/xenmods/minecraft-vcam) (or any WebSocket client on port 4322).

## Requirements

- Windows (for screen capture via gdigrab)
- Go 1.26+ (to build)
