//go:build !windows

package agent

func setBlockInput(bool) error {
	return errControlInvalid
}
