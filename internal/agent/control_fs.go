package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pion/webrtc/v4"
)

const fsMaxListEntries = 500

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
	return dcStreamFileToDC(dc, "fs_get", filepath.Base(clean), clean)
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
