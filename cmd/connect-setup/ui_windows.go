//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed web/index.html
var setupHTML string

func runUI(opts InstallOptions) error {
	runtime.LockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		if errno, ok := err.(windows.Errno); !ok || uint32(errno) != 1 {
			log.Printf("connect-setup: CoInitializeEx: %v", err)
		}
	}
	defer windows.CoUninitialize()

	dataPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "Connect", "setup-webview2")
	_ = os.MkdirAll(dataPath, 0o755)

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		DataPath:  dataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "WorthyJoin Setup",
			Width:  480,
			Height: 560,
			Center: true,
		},
	})
	if w == nil {
		// Fallback: quiet-style install if code provided, else error.
		if opts.Code != "" {
			return runInstall(opts, func(step, detail string) {
				fmt.Fprintf(os.Stderr, "%s: %s\n", step, detail)
			})
		}
		return fmt.Errorf("WebView2 is required to show Setup (install Microsoft Edge WebView2 Runtime), or run with -quiet -code …")
	}

	preset, _ := json.Marshal(map[string]string{
		"code":     opts.Code,
		"server":   opts.Server,
		"agentUrl": opts.AgentURL,
	})
	html := strings.Replace(setupHTML, "const preset = window.__PRESET__ || {};", "const preset = "+string(preset)+";", 1)

	_ = w.Bind("setupInstall", func(req map[string]any) map[string]any {
		code, _ := req["code"].(string)
		server, _ := req["server"].(string)
		agentURL, _ := req["agentUrl"].(string)
		o := InstallOptions{
			Code:     strings.TrimSpace(code),
			Server:   normalizeServer(server),
			AgentURL: strings.TrimSpace(agentURL),
		}
		if o.AgentURL == "" {
			o.AgentURL = agentURLFromServer(o.Server)
		}
		err := runInstall(o, func(step, detail string) {
			js := fmt.Sprintf("window.setupProgress(%q,%q)", step, detail)
			w.Dispatch(func() { w.Eval(js) })
		})
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		return map[string]any{"ok": true, "message": "Installed. This PC will appear online shortly."}
	})

	w.SetHtml(html)
	w.Run()
	return nil
}
