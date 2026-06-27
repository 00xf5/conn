//go:build !windows

package main

import (
	"log"

	"connect/internal/agent"
)

func runTray(a *agent.Agent, logPath string) {
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupFileLog() string { return "" }
