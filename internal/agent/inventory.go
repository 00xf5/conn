package agent

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"connect/internal/rendezvous"
)

const inventoryRefresh = 45 * time.Second

type invInput struct {
	cfg           Config
	sessionActive bool
	encoderName   string
}

func (a *Agent) heartbeatInventory() *rendezvous.HostInventory {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("agent: inventory panic recovered: %v", r)
		}
	}()

	a.mu.Lock()
	in := invInput{cfg: a.cfg, sessionActive: a.activeSess != ""}
	if a.enc != nil {
		in.encoderName = a.enc.Name()
	}
	a.mu.Unlock()

	// Never block the heartbeat/reconnect loop on registry/NIC sampling.
	// Return last cache immediately; refresh in the background when stale.
	a.invMu.Lock()
	cache := a.invCache
	stale := cache == nil || time.Since(a.invAt) >= inventoryRefresh
	if stale && !a.invRefresh {
		a.invRefresh = true
		a.invMu.Unlock()
		go a.refreshInventoryAsync(in)
	} else {
		a.invMu.Unlock()
	}

	if cache == nil {
		// First run: kick async refresh (above) and send heartbeat without inventory.
		return nil
	}
	out := *cache
	applyStreamFields(&out, in)
	return &out
}

func (a *Agent) refreshInventoryAsync(in invInput) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("agent: inventory refresh panic recovered: %v", r)
		}
		a.invMu.Lock()
		a.invRefresh = false
		a.invMu.Unlock()
	}()

	inv := collectInventory(in)

	a.invMu.Lock()
	defer a.invMu.Unlock()
	a.invCache = inv
	a.invAt = time.Now()
}

func collectInventory(in invInput) *rendezvous.HostInventory {
	inv := &rendezvous.HostInventory{
		Arch:         runtime.GOARCH,
		AgentVersion: agentVersionString(),
		ServerURL:    in.cfg.ServerURL,
		Monitor:      in.cfg.Monitor,
	}
	if host, err := os.Hostname(); err == nil {
		inv.FQDN = host
	}
	if inv.FQDN == "" {
		inv.FQDN = in.cfg.Hostname
	}
	fillPlatformInventory(inv)
	applyStreamFields(inv, in)
	return inv
}

func applyStreamFields(inv *rendezvous.HostInventory, in invInput) {
	inv.ServerURL = in.cfg.ServerURL
	inv.Monitor = in.cfg.Monitor
	inv.FPS = in.cfg.FPS
	inv.BitrateK = in.cfg.BitrateK
	inv.GOP = in.cfg.GOP
	if in.cfg.Width > 0 && in.cfg.Height > 0 {
		inv.Resolution = fmt.Sprintf("%dx%d", in.cfg.Width, in.cfg.Height)
	}
	if in.encoderName != "" {
		inv.Encoder = in.encoderName
	}
	active := in.sessionActive
	inv.SessionActive = &active
}

func agentVersionString() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		v := strings.TrimSpace(bi.Main.Version)
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return runtime.Version()
}
