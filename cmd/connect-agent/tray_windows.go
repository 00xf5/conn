//go:build windows

package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connect/internal/agent"
	"connect/internal/hostui"

	"github.com/getlantern/systray"
)

func runTray(a *agent.Agent, logPath string) {
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("WorthyJoin")
		systray.SetTooltip("WorthyJoin host agent")

		mStatus := systray.AddMenuItem("Starting…", "Connection status")
		mStatus.Disable()
		mSession := systray.AddMenuItem("No session", "Active session")
		mSession.Disable()
		systray.AddSeparator()
		mOpen := systray.AddMenuItem("Open Host app", "Open WorthyJoin Host (requires host key)")
		mDashboard := systray.AddMenuItem("Open dashboard", "Open WorthyJoin dashboard in browser")
		mOpenLog := systray.AddMenuItem("Open log file", "Open agent log")
		mQuit := systray.AddMenuItem("Quit WorthyJoin", "Stop the host agent")

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("connect-agent panic: %v", r)
				}
			}()
			if err := a.Run(); err != nil {
				log.Printf("connect-agent stopped: %v", err)
			}
		}()

		go func() {
			for {
				st := a.Snapshot()
				mStatus.SetTitle(fmt.Sprintf("Status: %s", st.State))
				if st.State == "streaming" && st.Session != "" {
					mSession.SetTitle("Session: " + st.Session)
				} else {
					mSession.SetTitle("No active session")
				}
				switch st.State {
				case "streaming":
					systray.SetTooltip("WorthyJoin — streaming " + st.Session)
				case "online":
					systray.SetTooltip("WorthyJoin — online, waiting for viewer")
				default:
					systray.SetTooltip("WorthyJoin — offline")
				}
				time.Sleep(2 * time.Second)
			}
		}()

		for {
			select {
			case <-mQuit.ClickedCh:
				a.Stop()
				systray.Quit()
				return
			case <-mOpen.ClickedCh:
				hostui.Show()
			case <-mOpenLog.ClickedCh:
				if logPath != "" {
					_ = shellOpen(logPath)
				}
			case <-mDashboard.ClickedCh:
				if dash := dashboardURL(a.Snapshot().Server); dash != "" {
					_ = shellOpen(dash)
				}
			}
		}
	}, func() {
		a.Stop()
	})
}

func dashboardURL(serverWS string) string {
	u, err := url.Parse(serverWS)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return ""
	}
	u.Path = "/dashboard/"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func setupFileLog() string {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Connect")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "agent.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	log.SetOutput(io.MultiWriter(f))
	log.Printf("connect-agent logging to %s", path)
	return path
}
