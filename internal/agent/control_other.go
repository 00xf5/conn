//go:build !windows

package agent

func controlCtrlAltDel() error              { return errControlInvalid }
func controlLock() error                     { return errControlInvalid }
func controlShutdown(bool) error             { return errControlInvalid }
func controlOpenURL(string) error            { return errControlInvalid }
func controlRun(string) error                { return errControlInvalid }
func controlClipboard(string) error          { return errControlInvalid }
func controlWinD() error                     { return errControlInvalid }

type fileEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func controlListDownloads() ([]fileEntry, error) { return nil, errControlInvalid }
