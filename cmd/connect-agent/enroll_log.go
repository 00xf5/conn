package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"connect/internal/agent"
)

func setupEnrollLog() string {
	dir := agent.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "enroll.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return ""
	}
	// Keep file open for process lifetime; installer may type this on failure.
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	return path
}

func fatalEnroll(logPath string, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte(msg+"\n"), 0o644)
	}
	os.Exit(1)
}
