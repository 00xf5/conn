package main

import (
	"flag"
	"log"
	"runtime"

	"connect/internal/agent"
)

func main() {
	serverURL := flag.String("server", "", "connectd WebSocket URL (overrides config.json)")
	deviceID := flag.String("device", "", "device ID (auto-generated if empty)")
	width := flag.Int("width", 0, "stream width (default from config or 854)")
	height := flag.Int("height", 0, "stream height (default from config or 480)")
	fps := flag.Int("fps", 0, "capture FPS (default from config or 20)")
	bitrate := flag.Int("bitrate", 0, "video bitrate kbps (default from config or 2000)")
	monitor := flag.Int("monitor", 0, "monitor index")
	insecureTLS := flag.Bool("insecure-tls", false, "skip TLS verify for self-signed connectd cert")
	console := flag.Bool("console", false, "show console window (Windows tray mode is default)")
	flag.Parse()

	exitIfAlreadyRunning()

	cli := agent.Config{
		ServerURL:   *serverURL,
		DeviceID:    *deviceID,
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
		log.Printf("connect-agent: loaded config.json (server=%s %dx%d @ %dfps %dkbps)",
			cfg.ServerURL, cfg.Width, cfg.Height, cfg.FPS, cfg.BitrateK)
	} else {
		if cfg.ServerURL == "" {
			cfg.ServerURL = "wss://localhost:8787/ws"
		}
		cfg = agent.NormalizeConfig(cfg)
	}

	a := agent.New(cfg)

	if *console {
		enableConsole()
		log.Printf("connect-agent starting (device=%s)", a.DeviceID())
		if err := a.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if runtime.GOOS == "windows" {
		logPath := setupFileLog()
		log.Printf("connect-agent starting (device=%s)", a.DeviceID())
		runTray(a, logPath)
		return
	}

	log.Printf("connect-agent starting (device=%s)", a.DeviceID())
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
