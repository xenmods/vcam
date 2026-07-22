# VCam

Control Minecraft using your phone — gyro, touch, and joystick for full camera override, with live screen stream.

Two components:

- **vcam.exe** — Windows launcher that downloads go2rtc + ffmpeg, generates a TLS cert, and serves the mobile web UI
- **vcam-mod.jar** — Fabric mod for Minecraft 1.21.1 that receives phone input over WebSocket and overrides the in-game camera

---

## Quick Start

### 1. Install the mod

Place `vcam-mod-<version>+mc1.21.1.jar` in your Minecraft `mods/` folder (requires Fabric Loader 0.16+ and Fabric API).

### 2. Run the bootstrap

On your Windows PC, run:

```powershell
vcam.exe
```

It downloads go2rtc + ffmpeg on first run. You'll be asked: *Enable screen capture? [Y/n]* — say `n` on low-end PCs.

### 3. Connect your phone

Open `https://<YOUR_LAN_IP>:4321/` on your phone (shown in the terminal).

### 4. Start tracking

- Press **START TRACKING** on the phone page
- Press **V** in Minecraft to enable camera override
- Move your phone to look around, swipe the right half of the screen for quick turns, use the joystick to move

---

## Downloads

Get the latest from [Releases](https://github.com/xenmods/vcam/releases).

| File | What it is |
|------|-----------|
| `vcam-<version>.exe` | Windows launcher (single binary, ~7 MB) |
| `vcam-mod-<version>+mc1.21.1.jar` | Fabric mod for Minecraft 1.21.1 |

---

## Building from source

### Bootstrap (Windows)

```powershell
go build -o vcam.exe -ldflags="-s -w" .
```

Requires Go 1.26+. The webapp is pre-built and embedded — no Node.js needed.

---

## Controls

| Input | Action |
|-------|--------|
| Phone gyro | Look around (yaw / pitch / roll) |
| Swipe right side of screen | Quick camera turns (FPS-style) |
| Bottom-left joystick | Dolly forward/backward/strafe |
| Pinch / scroll | Zoom FOV |
| ⚙ (top-right) | Settings: smoothing, sensitivity, invert pitch |

---

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 4321 | HTTPS | Web UI + video stream (go2rtc) |
| 4322 | WSS | Gyro / touch / joystick input (mod) |
| 8555 | UDP/TCP | WebRTC video (go2rtc) |

---

## Requirements

- **Bootstrap:** Windows 10+, internet for first-run downloads
- **Mod:** Minecraft 1.21.1, Fabric Loader 0.16+, Fabric API
- **Phone:** Modern browser (Chrome, Safari, Brave), iOS 16+ or Android 10+
