//go:build windows

package hostui

import (
	"bytes"
	_ "embed"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed web/shell.html.tmpl
var shellTmpl string

//go:embed web/app.css
var shellCSS string

//go:embed web/app.js
var shellJS string

var (
	showMu sync.Mutex
)

// Show launches WorthyJoin-Host.exe when present (desktop GUI), otherwise
// falls back to the same exe with -host-ui so older packages still work.
func Show() {
	showMu.Lock()
	defer showMu.Unlock()

	if uiAlreadyRunning() {
		log.Printf("hostui: window process already running")
		return
	}

	exe, err := os.Executable()
	if err != nil || exe == "" {
		log.Printf("hostui: executable path: %v", err)
		return
	}
	dir := filepath.Dir(exe)
	hostExe := filepath.Join(dir, "WorthyJoin-Host.exe")
	var cmd *exec.Cmd
	if st, err := os.Stat(hostExe); err == nil && !st.IsDir() {
		cmd = exec.Command(hostExe)
		cmd.Dir = dir
	} else {
		cmd = exec.Command(exe, "-host-ui")
		cmd.Dir = dir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// DETACHED_PROCESS — independent of tray lifetime for the window message loop.
		CreationFlags: 0x00000008,
	}
	if err := cmd.Start(); err != nil {
		log.Printf("hostui: start: %v", err)
		return
	}
	log.Printf("hostui: started ui pid=%d", cmd.Process.Pid)
	_ = cmd.Process.Release()
}

// RunBlocking runs the WebView2 shell on the current thread (use from -host-ui main).
func RunBlocking() {
	runtime.LockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		if errno, ok := err.(windows.Errno); !ok || uint32(errno) != 1 /* S_FALSE */ {
			// RPC_E_CHANGED_MODE — should not happen on a fresh process.
			log.Printf("hostui: CoInitializeEx: %v", err)
		}
	}
	defer windows.CoUninitialize()

	release, ok := acquireUIMutex()
	if !ok {
		log.Printf("hostui: another UI window is already open")
		return
	}
	defer release()

	if err := runWindow(); err != nil {
		log.Printf("hostui: %v", err)
	}
}

func runWindow() error {
	dataPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "Connect", "webview2")
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return err
	}

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		DataPath:  dataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "WorthyJoin Host",
			Width:  1100,
			Height: 720,
			Center: true,
		},
	})
	if w == nil {
		return errWebViewUnavailable
	}

	w.SetHtml(buildShellHTML())

	_ = w.Bind("hostStub", func(name string) string {
		log.Printf("hostui: stub action %q", name)
		return "coming_soon"
	})
	_ = w.Bind("hostHide", func() {
		w.Dispatch(func() { w.Terminate() })
	})
	_ = w.Bind("hostUnlockStatus", func() map[string]any {
		id := loadHostIdentity()
		return map[string]any{
			"unlocked": rememberedUnlocked(id.DeviceID),
			"deviceId": id.DeviceID,
			"hostname": id.Hostname,
			"enrolled": id.Enrolled,
		}
	})
	_ = w.Bind("hostUnlock", func(key string) map[string]any {
		if err := unlockWithKey(key); err != nil {
			log.Printf("hostui: unlock failed: %v", err)
			return map[string]any{"ok": false, "error": err.Error()}
		}
		log.Printf("hostui: unlocked")
		return map[string]any{"ok": true}
	})
	_ = w.Bind("hostLock", func() map[string]any {
		clearUnlockFile()
		log.Printf("hostui: locked")
		return map[string]any{"ok": true}
	})

	log.Printf("hostui: window open")
	w.Run()
	log.Printf("hostui: window closed")
	return nil
}

func buildShellHTML() string {
	html := shellTmpl
	html = strings.Replace(html, "__CSS__", shellCSS, 1)
	html = strings.Replace(html, "__JS__", shellJS, 1)
	return string(bytes.TrimPrefix([]byte(html), []byte{0xEF, 0xBB, 0xBF}))
}

func acquireUIMutex() (release func(), ok bool) {
	name, err := syscall.UTF16PtrFromString("Local\\WorthyJoinHostUI")
	if err != nil {
		return func() {}, true
	}
	mod := syscall.NewLazyDLL("kernel32.dll")
	create := mod.NewProc("CreateMutexW")
	handle, _, callErr := create.Call(0, 1, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return func() {}, true
	}
	h := syscall.Handle(handle)
	if callErr == syscall.Errno(183) || callErr == windows.ERROR_ALREADY_EXISTS { // ERROR_ALREADY_EXISTS
		_ = syscall.CloseHandle(h)
		return nil, false
	}
	return func() { _ = syscall.CloseHandle(h) }, true
}

func uiAlreadyRunning() bool {
	name, err := syscall.UTF16PtrFromString("Local\\WorthyJoinHostUI")
	if err != nil {
		return false
	}
	mod := syscall.NewLazyDLL("kernel32.dll")
	open := mod.NewProc("OpenMutexW")
	const synchronize = 0x00100000
	handle, _, _ := open.Call(synchronize, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return false
	}
	_ = syscall.CloseHandle(syscall.Handle(handle))
	return true
}

type unavailableError string

func (e unavailableError) Error() string { return string(e) }

const errWebViewUnavailable unavailableError = "WebView2 unavailable (install Edge WebView2 runtime)"
