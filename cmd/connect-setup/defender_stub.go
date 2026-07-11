//go:build !windows

package main

func unblockPath(string)                    {}
func unblockInstallTree(string)             {}
func unblockSetupBundle()                   {}
func ensureDefenderAllowsInstallDir(string) {}
