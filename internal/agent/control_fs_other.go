//go:build !windows

package agent

func controlFsRoots() ([]fsEntry, error) {
	return nil, errControlInvalid
}

func controlFsList(string) ([]fsEntry, string, error) {
	return nil, "", errControlInvalid
}
