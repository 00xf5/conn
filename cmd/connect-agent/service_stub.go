//go:build !windows

package main

func isWindowsService() bool { return false }

func runServiceMode() error { return nil }

func serviceInstalled() bool { return false }

func installService() error { return nil }

func uninstallService() error { return nil }
