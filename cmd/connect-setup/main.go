package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	code := flag.String("code", "", "enrollment code (ENR-…)")
	server := flag.String("server", "", "agent websocket URL (wss://host/ws)")
	agentURL := flag.String("agent-url", "", "agent package URL (https://host/download/agent.zip)")
	quiet := flag.Bool("quiet", false, "run install without UI (requires -code)")
	flag.Parse()

	opts := InstallOptions{
		Code:     strings.TrimSpace(*code),
		Server:   normalizeServer(*server),
		AgentURL: strings.TrimSpace(*agentURL),
		Quiet:    *quiet,
	}
	if opts.AgentURL == "" {
		opts.AgentURL = agentURLFromServer(opts.Server)
	}

	if *quiet {
		if opts.Code == "" {
			fmt.Fprintln(os.Stderr, "connect-setup: -quiet requires -code")
			os.Exit(2)
		}
		err := runInstall(opts, func(step, detail string) {
			fmt.Fprintf(os.Stderr, "%s: %s\n", step, detail)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runUI(opts); err != nil {
		fmt.Fprintf(os.Stderr, "connect-setup: %v\n", err)
		os.Exit(1)
	}
}
