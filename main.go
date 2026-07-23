package main

import (
	"archive/zip"
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed webapp/*
var webappFS embed.FS

const (
	workDirName   = ".vcam"
	go2rtcURL     = "https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_win64.zip"
	go2rtcExe     = "go2rtc.exe"
	go2rtcConfig  = "go2rtc.yaml"
	certFile      = "cert.pem"
	keyFile       = "key.pem"
	httpPort      = 4321
	apiPort       = 1984
	webrtcPort    = 8555
	wsPort        = 4322
)

var workDir string
var lanIP string
var captureEnabled = true

func main() {
	printBanner()

	workDir = getWorkDir()
	lanIP = getLocalIP()
	if lanIP == "" {
		lanIP = "localhost"
	}

	os.MkdirAll(workDir, 0755)

	if err := ensureGo2RTC(); err != nil {
		fatal("go2rtc", err)
	}
	logOK("go2rtc", "ready")

	captureEnabled = promptCapture()

	if captureEnabled {
		if err := ensureFFmpeg(); err != nil {
			logWarn("ffmpeg", err.Error()+". Install: winget install Gyan.FFmpeg.Essentials")
		} else {
			logOK("ffmpeg", "ready")
		}
	} else {
		logWarn("capture", "screen capture disabled by user")
	}

	if err := generateCert(); err != nil {
		fatal("TLS cert", err)
	}
	logOK("TLS", "certificate ready")

	if err := deployWebapp(); err != nil {
		logWarn("webapp", err.Error())
	} else {
		logOK("webapp", "deployed")
	}

	if err := writeConfig(); err != nil {
		fatal("config", err)
	}

	printConnectionInfo()

	if err := startGo2RTC(); err != nil {
		fatal("go2rtc", err)
	}
}

func printBanner() {
	fmt.Println()
	fmt.Println("  ╔════════════════════════════════════════╗")
	fmt.Println("  ║           VCam Server v1.0             ║")
	fmt.Println("  ╚════════════════════════════════════════╝")
	fmt.Println()
}

func printConnectionInfo() {
	fmt.Println("  ───────────────────────────────────────────")
	fmt.Println("   📱  https://" + lanIP + ":" + strconv.Itoa(httpPort) + "/")
	fmt.Println("   🎮  wss://" + lanIP + ":" + strconv.Itoa(wsPort))
	if !captureEnabled {
		fmt.Println()
		fmt.Println("   ⚠ Screen capture disabled (use Y/n on restart to enable)")
	}
	fmt.Println("  ───────────────────────────────────────────")
	fmt.Println("   Make sure Minecraft is running with VCam mod")
	fmt.Println("  ───────────────────────────────────────────")
	fmt.Println()
}

func getWorkDir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, workDirName)
}

func getLocalIP() string {
	type candidate struct {
		ip      string
		private bool
	}
	var candidates []candidate

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				candidates = append(candidates, candidate{
					ip:      ip4.String(),
					private: ip4.IsPrivate(),
				})
			}
		}
	}

	// Prefer private IPs (LAN), fall back to any non-loopback IP
	for _, c := range candidates {
		if c.private {
			return c.ip
		}
	}
	if len(candidates) > 0 {
		return candidates[0].ip
	}
	return ""
}

func fatal(component string, err error) {
	fmt.Printf("  ✗ %s: %v\n", component, err)
	fmt.Println("  Press Enter to exit...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}

func logOK(component, msg string) {
	fmt.Printf("  ✓ %-12s %s\n", component, msg)
}

func logWarn(component, msg string) {
	fmt.Printf("  ⚠ %-12s %s\n", component, msg)
}

func logInfo(msg string) {
	fmt.Println("  " + msg)
}

func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	_ = written
	return nil
}

func extractZip(zipPath, destDir, wantExe string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if !f.FileInfo().IsDir() && strings.EqualFold(base, wantExe) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(filepath.Join(destDir, wantExe))
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("%s not found in archive", wantExe)
}

func ensureGo2RTC() error {
	exePath := filepath.Join(workDir, go2rtcExe)
	if _, err := os.Stat(exePath); err == nil {
		return nil
	}

	tmpZip := filepath.Join(workDir, "go2rtc.zip")
	if err := downloadFile(go2rtcURL, tmpZip); err != nil {
		return err
	}
	defer os.Remove(tmpZip)

	return extractZip(tmpZip, workDir, go2rtcExe)
}

const ffmpegURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

func ffmpegSizeOK(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 10*1024*1024
}

func findFFmpeg() string {
	// Check work dir
	p := filepath.Join(workDir, "ffmpeg.exe")
	if ffmpegSizeOK(p) {
		return p
	}
	// Check old temp location
	p = filepath.Join(os.TempDir(), "opencode", "vcam", "ffmpeg.exe")
	if ffmpegSizeOK(p) {
		return p
	}
	// Check PATH
	if pp, err := exec.LookPath("ffmpeg"); err == nil && ffmpegSizeOK(pp) {
		return pp
	}
	// Check winget location
	locals := os.Getenv("LOCALAPPDATA")
	if locals != "" {
		matches, _ := filepath.Glob(filepath.Join(locals, "Microsoft", "WinGet", "Packages", "Gyan.FFmpeg.Essentials*", "ffmpeg.exe"))
		for _, m := range matches {
			if ffmpegSizeOK(m) {
				return m
			}
		}
	}
	return ""
}

func ensureFFmpeg() error {
	exePath := filepath.Join(workDir, "ffmpeg.exe")
	if ffmpegSizeOK(exePath) {
		return nil
	}

	if src := findFFmpeg(); src != "" {
		return copyFile(src, exePath)
	}

	// Auto-download ffmpeg
	fmt.Println()
	logInfo("Downloading ffmpeg (~70 MB)...")
	tmpZip := filepath.Join(workDir, "ffmpeg.zip")
	if err := downloadFile(ffmpegURL, tmpZip); err != nil {
		os.Remove(tmpZip)
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpZip)

	r, err := zip.OpenReader(tmpZip)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if !f.FileInfo().IsDir() && strings.EqualFold(base, "ffmpeg.exe") && strings.Contains(f.Name, "bin/") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(exePath)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("ffmpeg.exe not found in downloaded archive")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func generateCert() error {
	certPath := filepath.Join(workDir, certFile)
	keyPath := filepath.Join(workDir, keyFile)

	if existing := loadExistingCert(certPath, keyPath); existing {
		return nil
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"VCam"},
			CommonName:   "localhost",
		},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	if ip := net.ParseIP(lanIP); ip != nil && !ip.IsLoopback() {
		template.IPAddresses = append(template.IPAddresses, ip)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	if err := writePEM(certPath, "CERTIFICATE", certDER); err != nil {
		return err
	}
	if err := writePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)); err != nil {
		return err
	}
	return nil
}

func loadExistingCert(certPath, keyPath string) bool {
	if _, err := os.Stat(certPath); err != nil {
		return false
	}
	if _, err := os.Stat(keyPath); err != nil {
		return false
	}
	certBytes, _ := os.ReadFile(certPath)
	if block, _ := pem.Decode(certBytes); block != nil {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return false
		}
		if !time.Now().Before(cert.NotAfter) {
			return false
		}
		// Verify the cert's IP SANs include the current LAN IP
		if ip := net.ParseIP(lanIP); ip != nil && !ip.IsLoopback() {
			for _, certIP := range cert.IPAddresses {
				if certIP.Equal(ip) {
					return true
				}
			}
			return false // LAN IP not in cert SANs
		}
		return true
	}
	return false
}

func writePEM(path, blockType string, derBytes []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: derBytes})
}

func deployWebapp() error {
	dest := filepath.Join(workDir, "webapp")
	os.MkdirAll(dest, 0755)

	entries, err := webappFS.ReadDir("webapp")
	if err != nil {
		return fmt.Errorf("reading embedded webapp: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("webapp directory is empty — run build.ps1 first")
	}

	return deployDir(webappFS, "webapp", dest, entries)
}

func deployDir(fs embed.FS, path, dest string, entries []os.DirEntry) error {
	for _, entry := range entries {
		srcPath := filepath.ToSlash(filepath.Join(path, entry.Name()))
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			os.MkdirAll(destPath, 0755)
			sub, err := fs.ReadDir(srcPath)
			if err != nil {
				return err
			}
			if err := deployDir(fs, srcPath, destPath, sub); err != nil {
				return err
			}
		} else {
			data, err := fs.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func promptCapture() bool {
	fmt.Print("\n  Enable screen capture? (show your PC desktop on the phone) [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "n" || line == "N" || line == "no" || line == "No" || line == "NO" {
		return false
	}
	return true
}

func writeConfig() error {
	ffmpegBin := strings.ReplaceAll(filepath.Join(workDir, "ffmpeg.exe"), "\\", "/")
	staticDir := strings.ReplaceAll(filepath.Join(workDir, "webapp"), "\\", "/")
	certPath := strings.ReplaceAll(filepath.Join(workDir, certFile), "\\", "/")
	keyPath := strings.ReplaceAll(filepath.Join(workDir, keyFile), "\\", "/")

	streamSection := ""
	if captureEnabled {
		streamSection = fmt.Sprintf(`ffmpeg:
  bin: %s
  gdigrab: -framerate 15 -f gdigrab -i {input}

streams:
  mc: ffmpeg:desktop#video=h264#input=gdigrab

`, ffmpegBin)
	}

	yaml := fmt.Sprintf(`%sapi:
  listen: ":%d"
  tls_listen: ":%d"
  tls_cert: %s
  tls_key: %s
  origin: "*"
  static_dir: %s

webrtc:
  candidates:
    - %s:%d
`, streamSection, apiPort, httpPort, certPath, keyPath, staticDir, lanIP, webrtcPort)

	return os.WriteFile(filepath.Join(workDir, go2rtcConfig), []byte(yaml), 0644)
}

func startGo2RTC() error {
	cmd := exec.Command(filepath.Join(workDir, go2rtcExe), "-c", filepath.Join(workDir, go2rtcConfig))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	fmt.Printf("  ▶ go2rtc running (PID %d)\n", cmd.Process.Pid)
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()

	done := make(chan struct{}, 2)
	go pipeOutput(stdout, done)
	go pipeOutput(stderr, done)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh
	fmt.Println()
	fmt.Println("  Shutting down...")
	cmd.Process.Signal(os.Interrupt)

	select {
	case <-done:
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
	}

	return nil
}

func pipeOutput(rc io.ReadCloser, done chan<- struct{}) {
	defer rc.Close()
	defer func() { done <- struct{}{} }()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 65536), 65536)
	for scanner.Scan() {
		fmt.Println("  " + scanner.Text())
	}
}
