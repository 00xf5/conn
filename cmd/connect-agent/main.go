package main

import (
	"flag"
	"log"
	"os"
	"runtime"

	"connect/internal/agent"
)

func main() {
	serverURL := flag.String("server", "", "connectd WebSocket URL (overrides config.json)")
	deviceID := flag.String("device", "", "device ID (auto-generated if empty)")
	tenantID := flag.String("tenant", "", "tenant ID (binds agent to Access tenant)")
	enroll := flag.String("enroll", "", "one-time enrollment code (ENR-…); binds tenant and saves config")
	width := flag.Int("width", 0, "stream width (default from config or 854)")
	height := flag.Int("height", 0, "stream height (default from config or 480)")
	fps := flag.Int("fps", 0, "capture FPS (default from config or 20)")
	bitrate := flag.Int("bitrate", 0, "video bitrate kbps (default from config or 2000)")
	monitor := flag.Int("monitor", 0, "monitor index")
	insecureTLS := flag.Bool("insecure-tls", false, "skip TLS verify for self-signed connectd cert")
	console := flag.Bool("console", false, "show console window (Windows tray mode is default)")
	service := flag.Bool("service", false, "run as Windows Service supervisor (SCM)")
	installSvc := flag.Bool("install-service", false, "install + start Windows Service (Administrator)")
	uninstallSvc := flag.Bool("uninstall-service", false, "stop + remove Windows Service (Administrator)")
	quitAfterEnroll := flag.Bool("quit-after-enroll", false, "exit after successful -enroll (installer use)")
	flag.Parse()

	// SCM entry — must not take the interactive single-instance mutex.
	if *service || isWindowsService() {
		if err := runServiceMode(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *uninstallSvc {
		if err := uninstallService(); err != nil {
			log.Fatal(err)
		}
		log.Printf("connect-agent: service %q removed", serviceNameOrFallback())
		return
	}

	cli := agent.Config{
		ServerURL:   *serverURL,
		DeviceID:    *deviceID,
		TenantID:    *tenantID,
		Monitor:     *monitor,
		Width:       *width,
		Height:      *height,
		FPS:         *fps,
		BitrateK:    *bitrate,
		InsecureTLS: *insecureTLS,
	}

	cfg := cli
	if fileCfg, ok := agent.LoadConfigFile(); ok {
		cfg = agent.MergeConfig(fileCfg, cli)
		log.Printf("connect-agent: loaded config.json (server=%s tenant=%s %dx%d @ %dfps %dkbps)",
			cfg.ServerURL, cfg.TenantID, cfg.Width, cfg.Height, cfg.FPS, cfg.BitrateK)
	} else {
		if cfg.ServerURL == "" {
			cfg.ServerURL = "wss://localhost:8787/ws"
		}
		cfg = agent.NormalizeConfig(cfg)
	}

	// Enroll / install-service must run BEFORE the single-instance mutex.
	// Otherwise a service-relaunched agent holds the mutex and this process
	// exits 0 without enrolling — install looks successful but dashboard stays empty.
	if *enroll != "" {
		if cfg.ServerURL == "" {
			log.Fatal("connect-agent: -enroll requires -server or serverUrl in config")
		}
		probe := agent.New(cfg)
		cfg.DeviceID = probe.DeviceID()
		host := cfg.Hostname
		if host == "" {
			host, _ = os.Hostname()
		}
		tid, tname, err := agent.EnrollWithCode(cfg.ServerURL, *enroll, cfg.DeviceID, host, cfg.InsecureTLS)
		if err != nil {
			log.Fatalf("connect-agent: enroll: %v", err)
		}
		cfg.TenantID = tid
		cfg.Hostname = host
		if err := agent.SaveConfigFile(cfg); err != nil {
			log.Fatalf("connect-agent: save config: %v", err)
		}
		log.Printf("connect-agent: enrolled tenant=%s (%s); config saved", tname, tid)
		if *quitAfterEnroll && !*installSvc {
			return
		}
	}

	if *installSvc {
		if err := installService(); err != nil {
			log.Fatalf("connect-agent: install-service: %v", err)
		}
		log.Printf("connect-agent: Windows Service installed and started")
		// Service launches the interactive agent in the user session.
		return
	}

	exitIfAlreadyRunning()

	a := agent.New(cfg)
	// Fallback persistence when Service is not installed (non-admin hosts).
	agent.EnsurePersistence()

	if *console {
		enableConsole()
		log.Printf("connect-agent starting (device=%s tenant=%s)", a.DeviceID(), cfg.TenantID)
		if err := a.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if runtime.GOOS == "windows" {
		logPath := setupFileLog()
		log.Printf("connect-agent starting (device=%s tenant=%s)", a.DeviceID(), cfg.TenantID)
		runTray(a, logPath)
		return
	}

	log.Printf("connect-agent starting (device=%s tenant=%s)", a.DeviceID(), cfg.TenantID)
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

func serviceNameOrFallback() string {
	if runtime.GOOS == "windows" {
		return "ConnectAgent"
	}
	return "ConnectAgent"
}
