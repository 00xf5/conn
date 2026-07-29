//go:build !windows

package agent

func startHostShell(cols, rows int) (*hostShell, error) {
	return nil, errControlInvalid
}
