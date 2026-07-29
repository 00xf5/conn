//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"strings"
)

func controlFsRoots() ([]fsEntry, error) {
	out := make([]fsEntry, 0, 32)

	addShortcut := func(label, path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			return
		}
		out = append(out, fsEntry{Name: label, Path: path, Dir: true})
	}

	home := os.Getenv("USERPROFILE")
	addShortcut("Home", home)
	addShortcut("Desktop", filepath.Join(home, "Desktop"))
	addShortcut("Documents", filepath.Join(home, "Documents"))
	addShortcut("Downloads", filepath.Join(home, "Downloads"))

	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			out = append(out, fsEntry{Name: root, Path: root, Dir: true})
		}
	}
	return out, nil
}

func controlFsList(path string) ([]fsEntry, string, error) {
	if strings.TrimSpace(path) == "" {
		roots, err := controlFsRoots()
		return roots, "", err
	}
	clean, err := sanitizeFsPath(path)
	if err != nil {
		return nil, "", err
	}
	entries, err := listFsDir(clean)
	return entries, clean, err
}
