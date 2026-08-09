# VCam

[![Showcase](https://img.youtube.com/vi/EzAjuX5gfyM/maxresdefault.jpg)](https://www.youtube.com/watch?v=EzAjuX5gfyM)

Control Minecraft using your phone — gyro, touch, and joystick for full camera override, with live screen stream.

Two components:

- **vcam** — Launcher that downloads go2rtc + ffmpeg, generates a TLS cert, and serves the mobile web UI
- **vcam-mod.jar** — Fabric mod for Minecraft 1.21.1 that receives phone input over WebSocket and overrides the in-game camera

---

## Quick Start

### 1. Install the mod

Place `vcam-mod-<version>+mc1.21.1.jar` in your Minecraft `mods/` folder (requires Fabric Loader 0.16+ and Fabric API).

### 2. Run the launcher

**Windows:**
```powershell
vcam.exe
```

**Linux:**
```bash
chmod +x vcam
./vcam
```

It downloads go2rtc + ffmpeg on first run. You'll be asked: *Enable screen capture? [Y/n]* — say `n` on low-end PCs.

### 3. Connect your phone

Open `https://<YOUR_LAN_IP>:4321/` on your phone (shown in the terminal + QR code).

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
| `vcam-<version>-linux-amd64` | Linux launcher (x86_64) |
| `vcam-<version>-linux-arm64` | Linux launcher (ARM64 / Raspberry Pi) |
| `vcam-mod-<version>+mc1.21.1.jar` | Fabric mod for Minecraft 1.21.1 |

---

## Building from source

### Windows

```powershell
cd app
.\build.ps1
```

### Linux

```bash
cd app
./build.sh
```

Requires Go 1.26+ and Node.js for the webapp build. The webapp is embedded into the binary.

### Cross-compile

```bash
# Linux from Windows
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o vcam-linux-amd64 -ldflags="-s -w" .
$env:GOOS="linux"; $env:GOARCH="arm64"; go build -o vcam-linux-arm64 -ldflags="-s -w" .

# Windows from Linux
GOOS=windows GOARCH=amd64 go build -o vcam.exe -ldflags="-s -w" .
```

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
| 4321 | HTTPS | Everything — web UI, video stream, gyro input |
| 8555 | UDP/TCP | WebRTC video (go2rtc) |

All traffic goes through a single HTTPS port. No need to open multiple ports.

---

## Troubleshooting

### "Connecting..." forever / can't reach mod

- **Is Minecraft running with the VCam mod?** The mod must be loaded for the WebSocket server to be available.
- **Firewall:** Make sure port **4321** and **8555** are allowed in your firewall for both Private and Public network profiles.
- **VPN / virtual adapters:** If you have WSL, Docker, Hyper-V, or a VPN running, the launcher might detect the wrong IP. Check the terminal output — the IP shown should match your actual WiFi/Ethernet IP.

### Black screen (no video)

- **Screen capture:** Did you answer **Y** to "Enable screen capture?" when starting vcam? Restart and say Y.
- **ffmpeg:** Make sure ffmpeg is installed. On Windows it auto-downloads; on Linux install via `sudo apt install ffmpeg` or your package manager.
- **WebRTC port:** Port **8555** (UDP) must be open for video streaming.
- **Router AP isolation:** Some routers block device-to-device traffic on WiFi (common on guest networks). Try a different network or disable AP isolation.

### Linux-specific

- **ffmpeg:** Install via your package manager: `sudo apt install ffmpeg` (Debian/Ubuntu) or `sudo pacman -S ffmpeg` (Arch).
- **Screen capture uses X11:** The `x11grab` capture method works on X11. Wayland users may need additional setup.
- **Config location:** VCam stores its config in `~/.config/vcam/` (or `$XDG_CONFIG_HOME/vcam/`).

---

## Requirements

- **Launcher:** Windows 10+ or Linux (x86_64 / ARM64), internet for first-run downloads
- **Mod:** Minecraft 1.21.1, Fabric Loader 0.16+, Fabric API
- **Phone:** Modern browser (Chrome, Safari, Brave), iOS 16+ or Android 10+
