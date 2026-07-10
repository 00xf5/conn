//go:build !windows

package hostui

// Show is a no-op on non-Windows builds.
func Show() {}

// RunBlocking is a no-op on non-Windows builds.
func RunBlocking() {}
