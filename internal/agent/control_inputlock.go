package agent

import "sync"

var (
	inputLockMu sync.Mutex
	inputLocked bool
)

func controlBlockLocalInput(lock bool) (bool, error) {
	inputLockMu.Lock()
	defer inputLockMu.Unlock()
	if err := setBlockInput(lock); err != nil {
		return inputLocked, err
	}
	inputLocked = lock
	return inputLocked, nil
}

// forceUnlockLocalInput restores host keyboard/mouse. Safe to call anytime
// (session end, disconnect, agent stop). Never panics; ignores errors.
func forceUnlockLocalInput() {
	inputLockMu.Lock()
	defer inputLockMu.Unlock()
	_ = setBlockInput(false)
	inputLocked = false
}

func localInputLocked() bool {
	inputLockMu.Lock()
	defer inputLockMu.Unlock()
	return inputLocked
}
