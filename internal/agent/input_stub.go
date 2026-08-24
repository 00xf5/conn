//go:build !windows

package agent

func applyInputWindows(data []byte, capW, capH int) {}

func injectText(s string) error { return errControlInvalid }
