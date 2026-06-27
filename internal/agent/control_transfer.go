package agent

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

type fileRx struct {
	name string
	size int64
	f    *os.File
}

var (
	fileRxMu sync.Mutex
	fileRxSt *fileRx
)

func (a *Agent) controlSendFile(name string, dc *webrtc.DataChannel) error {
	path, err := safeDownloadPath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const chunk = 48 * 1024
	for i, off := 0, 0; off < len(data); i, off = i+1, off+chunk {
		end := off + chunk
		if end > len(data) {
			end = len(data)
		}
		part := map[string]any{
			"type":   "control_result",
			"action": "download_file",
			"name":   filepath.Base(path),
			"idx":    i,
			"done":   end >= len(data),
			"data":   base64.StdEncoding.EncodeToString(data[off:end]),
		}
		raw, _ := json.Marshal(part)
		if err := dc.SendText(string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func controlFileBegin(name string, size int64) error {
	if name == "" {
		return errControlInvalid
	}
	name = filepath.Base(strings.ReplaceAll(name, `\`, `/`))
	dir, err := connectTransferDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	fileRxMu.Lock()
	defer fileRxMu.Unlock()
	if fileRxSt != nil {
		_ = fileRxSt.f.Close()
		fileRxSt = nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	fileRxSt = &fileRx{name: path, size: size, f: f}
	return nil
}

func controlFileChunk(_ int, b64 string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	fileRxMu.Lock()
	defer fileRxMu.Unlock()
	if fileRxSt == nil || fileRxSt.f == nil {
		return errControlInvalid
	}
	_, err = fileRxSt.f.Write(data)
	return err
}

func controlFileEnd() (string, error) {
	fileRxMu.Lock()
	defer fileRxMu.Unlock()
	if fileRxSt == nil || fileRxSt.f == nil {
		return "", errControlInvalid
	}
	path := fileRxSt.name
	_ = fileRxSt.f.Close()
	fileRxSt = nil
	return path, nil
}

func connectTransferDir() (string, error) {
	dir := filepath.Join(os.Getenv("USERPROFILE"), "Downloads", "Connect")
	return dir, os.MkdirAll(dir, 0o755)
}

func safeDownloadPath(name string) (string, error) {
	name = filepath.Base(strings.ReplaceAll(name, `\`, `/`))
	dir, err := connectTransferDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dir)) {
		return "", errControlInvalid
	}
	return path, nil
}
