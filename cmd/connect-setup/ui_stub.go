//go:build !windows

package main

import "fmt"

func runUI(opts InstallOptions) error {
	return fmt.Errorf("WorthyJoin Setup GUI is Windows-only; use: connect-setup -quiet -code … -server …")
}
