package main

import (
	"log"
	"runtime"

	"connect/internal/hostui"
)

// WorthyJoin-Host — separate Host GUI (desktop shortcut). Does not run the agent.
func main() {
	if runtime.GOOS != "windows" {
		log.Fatal("WorthyJoin Host GUI is Windows-only")
	}
	hostui.RunBlocking()
}
