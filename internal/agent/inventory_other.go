//go:build !windows

package agent

import (
	"os/user"
	"runtime"

	"connect/internal/rendezvous"
)

func fillPlatformInventory(inv *rendezvous.HostInventory) {
	inv.OS = runtime.GOOS
	inv.CPUCores = runtime.NumCPU()
	if u, err := user.Current(); err == nil && u != nil {
		inv.User = u.Username
	}
}

func refreshLiveInventory(inv *rendezvous.HostInventory) {}
