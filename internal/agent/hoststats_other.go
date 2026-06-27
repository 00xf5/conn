//go:build !windows

package agent

func sampleHostMetrics() hostMetrics {
	return hostMetrics{CPUCores: 1}
}

func sampleProcesses(_ uint64) []ProcessSnapshot { return nil }

func logicalCPUs() int { return 1 }
