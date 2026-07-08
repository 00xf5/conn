//go:build !windows || !cgo

package agent

import "fmt"

func openHostPipelineEncoder(cfg Config) (*hostPipelineEncoder, error) {
	_ = cfg
	return nil, fmt.Errorf("host pipeline requires Windows CGO build")
}

type hostPipelineEncoder struct{}
