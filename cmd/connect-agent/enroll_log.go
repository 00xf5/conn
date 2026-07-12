package main

import (
	"fmt"
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
	// File only — installer runs a windowsgui agent with no console; don't depend on stderr.
	log.SetOutput(f)
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
