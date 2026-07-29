package agent

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pion/webrtc/v4"
)

const (
	fsMaxGetBytes   = 50 << 20 // 50 MiB
	fsMaxListEntries = 500
)

type fsEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size,omitempty"`
	Mtime int64  `json:"mtime,omitempty"` // unix seconds
}

func (a *Agent) controlFsGet(path string, dc *webrtc.DataChannel) error {
	clean, err := sanitizeFsPath(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(clean)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errString("path is a directory")
	}
	if info.Size() > fsMaxGetBytes {
		return errString("file too large (max 50 MB)")
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return err
	}
	const chunk = 48 * 1024
	base := filepath.Base(clean)
	sendPart := func(i, off, end int, done bool) error {
		part := map[string]any{
			"type":   "control_result",
			"action": "fs_get",
			"name":   base,
			"path":   clean,
			"idx":    i,
			"done":   done,
			"data":   base64.StdEncoding.EncodeToString(data[off:end]),
		}
		raw, _ := json.Marshal(part)
		return dc.SendText(string(raw))
	}
	if len(data) == 0 {
		return sendPart(0, 0, 0, true)
	}
	for i, off := 0, 0; off < len(data); i, off = i+1, off+chunk {
		end := off + chunk
		if end > len(data) {
			end = len(data)
		}
		if err := sendPart(i, off, end, end >= len(data)); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeFsPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errControlInvalid
	}
	// Reject NUL and other junk early.
	if strings.ContainsRune(path, 0) {
		return "", errControlInvalid
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", errString("path must be absolute")
	}
	// After Clean, ".." should not remain except as volume-relative oddities.
	if strings.Contains(clean, "..") {
		return "", errControlInvalid
	}
	return clean, nil
}

func listFsDir(path string) ([]fsEntry, error) {
	clean, err := sanitizeFsPath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, err
	}
	out := make([]fsEntry, 0, min(len(entries), fsMaxListEntries))
	for i, e := range entries {
		if i >= fsMaxListEntries {
			break
		}
		full := filepath.Join(clean, e.Name())
		ent := fsEntry{
			Name: e.Name(),
			Path: full,
			Dir:  e.IsDir(),
		}
		if info, err := e.Info(); err == nil {
			ent.Size = info.Size()
			ent.Mtime = info.ModTime().Unix()
			ent.Dir = info.IsDir()
		}
		out = append(out, ent)
	}
	return out, nil
}
