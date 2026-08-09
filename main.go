package main

import (
	"archive/zip"
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

//go:embed webapp/*
var webappFS embed.FS

const (
	httpPort     = 4321
	apiPort      = 1984
	webrtcPort   = 8555
	certFile     = "cert.pem"
	keyFile      = "key.pem"
	go2rtcConfig = "go2rtc.yaml"
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
			logWarn("ffmpeg", err.Error()+". Install via your package manager or winget")
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

	cmd, err := startGo2RTC()
	if err != nil {
		fatal("go2rtc", err)
	}

	time.Sleep(500 * time.Millisecond)

	if err := startHTTPSProxy(); err != nil {
		fmt.Printf("  HTTPS proxy error: %v\n", err)
	}

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
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
	url := "https://" + lanIP + ":" + strconv.Itoa(httpPort) + "/"
	fmt.Println("  ───────────────────────────────────────────")
	fmt.Println("   📱  " + url)
	if !captureEnabled {
		fmt.Println()
		fmt.Println("   ⚠ Screen capture disabled (use Y/n on restart to enable)")
	}
	fmt.Println("  ───────────────────────────────────────────")
	fmt.Println("   Make sure Minecraft is running with VCam mod")
	fmt.Println("  ═══════════════════════════════════════════")
	qr, err := qrcode.New(url, qrcode.Medium)
	if err == nil {
		fmt.Println(qr.ToSmallString(false))
	}
	fmt.Println("  ═══════════════════════════════════════════")
	fmt.Println("   Scan QR code with your phone camera")
	fmt.Println()
}

func getWorkDir() string {
	var base string
	if runtime.GOOS == "windows" {
		base = os.Getenv("APPDATA")
		if base == "" {
			base = os.TempDir()
		}
		return filepath.Join(base, ".vcam")
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		base = xdg
	} else {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "vcam")
}

func go2rtcBinaryName() string {
	if runtime.GOOS == "windows" {
		return "go2rtc.exe"
	}
	return "go2rtc"
}

func go2rtcDownloadURL() string {
	if runtime.GOOS == "windows" {
		return "https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_win64.zip"
	}
	if runtime.GOARCH == "arm64" {
		return "https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_arm64"
	}
	return "https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_amd64"
}

func ffmpegBinaryName() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
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

	_, err = io.Copy(out, resp.Body)
	return err
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
	exePath := filepath.Join(workDir, go2rtcBinaryName())
	if _, err := os.Stat(exePath); err == nil {
		return nil
	}

	if runtime.GOOS == "windows" {
		tmpZip := filepath.Join(workDir, "go2rtc.zip")
		if err := downloadFile(go2rtcDownloadURL(), tmpZip); err != nil {
			return err
		}
		defer os.Remove(tmpZip)

		if err := extractZip(tmpZip, workDir, go2rtcBinaryName()); err != nil {
			return err
		}
	} else {
		if err := downloadFile(go2rtcDownloadURL(), exePath); err != nil {
			return err
		}
	}

	return os.Chmod(exePath, 0755)
}

const ffmpegURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

func ffmpegSizeOK(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 10*1024*1024
}

func findFFmpeg() string {
	binName := ffmpegBinaryName()
	p := filepath.Join(workDir, binName)
	if ffmpegSizeOK(p) {
		return p
	}
	p = filepath.Join(os.TempDir(), "opencode", "vcam", binName)
	if ffmpegSizeOK(p) {
		return p
	}
	if pp, err := exec.LookPath("ffmpeg"); err == nil && ffmpegSizeOK(pp) {
		return pp
	}
	if runtime.GOOS == "windows" {
		locals := os.Getenv("LOCALAPPDATA")
		if locals != "" {
			matches, _ := filepath.Glob(filepath.Join(locals, "Microsoft", "WinGet", "Packages", "Gyan.FFmpeg.Essentials*", "ffmpeg.exe"))
			for _, m := range matches {
				if ffmpegSizeOK(m) {
					return m
				}
			}
		}
	}
	return ""
}

func ensureFFmpeg() error {
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			return fmt.Errorf("ffmpeg not found in PATH. Please install it (e.g. sudo apt install ffmpeg)")
		}
		return nil
	}

	exePath := filepath.Join(workDir, "ffmpeg.exe")
	if ffmpegSizeOK(exePath) {
		return nil
	}

	if src := findFFmpeg(); src != "" {
		return copyFile(src, exePath)
	}

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
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", keyDER); err != nil {
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
		if ip := net.ParseIP(lanIP); ip != nil && !ip.IsLoopback() {
			for _, certIP := range cert.IPAddresses {
				if certIP.Equal(ip) {
					return true
				}
			}
			return false
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
	streamSection := ""
	if captureEnabled {
		if runtime.GOOS == "windows" {
			ffmpegBin := strings.ReplaceAll(filepath.Join(workDir, "ffmpeg.exe"), "\\", "/")
			streamSection = fmt.Sprintf(`ffmpeg:
  bin: %s
  gdigrab: -framerate 15 -f gdigrab -i {input}

streams:
  mc: ffmpeg:desktop#video=h264#input=gdigrab

`, ffmpegBin)
		} else {
			ffmpegPath, _ := exec.LookPath("ffmpeg")
			if ffmpegPath == "" {
				ffmpegPath = "ffmpeg"
			}
			streamSection = fmt.Sprintf(`ffmpeg:
  bin: %s
  x11grab: -framerate 15 -f x11grab -i {input}

streams:
  mc: ffmpeg::0.0#video=h264#input=x11grab

`, ffmpegPath)
		}
	}

	yaml := fmt.Sprintf(`%sapi:
  listen: "localhost:%d"
  origin: "*"

webrtc:
  candidates:
    - %s:%d
`, streamSection, apiPort, lanIP, webrtcPort)

	return os.WriteFile(filepath.Join(workDir, go2rtcConfig), []byte(yaml), 0644)
}

func startGo2RTC() (*exec.Cmd, error) {
	cmd := exec.Command(filepath.Join(workDir, go2rtcBinaryName()), "-c", filepath.Join(workDir, go2rtcConfig))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	fmt.Printf("  ▶ go2rtc running (PID %d)\n", cmd.Process.Pid)
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()

	done := make(chan struct{}, 2)
	go pipeOutput(stdout, done)
	go pipeOutput(stderr, done)

	return cmd, nil
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

func startHTTPSProxy() error {
	certPath := filepath.Join(workDir, certFile)
	keyPath := filepath.Join(workDir, keyFile)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		proxyWebSocket(w, r, "localhost:4323")
	})

	go2rtcURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", apiPort))
	go2rtcProxy := httputil.NewSingleHostReverseProxy(go2rtcURL)

	go2rtcHandler := func(w http.ResponseWriter, r *http.Request) {
		if isWebSocketUpgrade(r) {
			proxyWebSocket(w, r, fmt.Sprintf("localhost:%d", apiPort))
		} else {
			go2rtcProxy.ServeHTTP(w, r)
		}
	}

	mux.HandleFunc("/api/", go2rtcHandler)
	mux.HandleFunc("/stream.html", go2rtcHandler)

	webappHandler := http.FileServer(http.Dir(filepath.Join(workDir, "webapp")))
	mux.Handle("/", webappHandler)

	srv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", httpPort),
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	go func() {
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			fatal("HTTPS Proxy", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\n  Shutting down HTTPS server...")
	return srv.Close()
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

func proxyWebSocket(w http.ResponseWriter, r *http.Request, targetHost string) {
	if !isWebSocketUpgrade(r) {
		http.Error(w, "Not a websocket upgrade", http.StatusBadRequest)
		return
	}

	targetConn, err := net.Dial("tcp", targetHost)
	if err != nil {
		http.Error(w, "Could not connect to backend", http.StatusBadGateway)
		return
	}
	defer targetConn.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	if err := r.Write(targetConn); err != nil {
		return
	}

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(targetConn, bufrw)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, targetConn)
		errc <- err
	}()

	<-errc
}
