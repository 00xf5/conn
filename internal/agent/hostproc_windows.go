//go:build windows

package agent

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	processQueryLimited = 0x1000
	processVMRead       = 0x0010
	processQueryInfo    = 0x0400
)

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var (
	procSampleMu   sync.Mutex
	lastProcSample time.Time
	lastProcCPU    map[uint32]uint64
)

func basenameExe(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "?"
	}
	return filepath.Base(path)
}

func processImageName(pid uint32) string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProc := kernel32.NewProc("OpenProcess")
	queryName := kernel32.NewProc("QueryFullProcessImageNameW")
	closeHandle := kernel32.NewProc("CloseHandle")

	h, _, _ := openProc.Call(processQueryLimited|processVMRead, 0, uintptr(pid))
	if h == 0 {
		return "?"
	}
	defer closeHandle.Call(h)

	buf := make([]uint16, syscall.MAX_PATH)
	size := uint32(len(buf))
	r, _, _ := queryName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return "?"
	}
	return basenameExe(syscall.UTF16ToString(buf[:size]))
}

func processMemoryMB(pid uint32) float64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	psapi := syscall.NewLazyDLL("psapi.dll")
	openProc := kernel32.NewProc("OpenProcess")
	getMem := psapi.NewProc("GetProcessMemoryInfo")
	closeHandle := kernel32.NewProc("CloseHandle")

	h, _, _ := openProc.Call(processQueryInfo|processVMRead, 0, uintptr(pid))
	if h == 0 {
		return 0
	}
	defer closeHandle.Call(h)

	var counters processMemoryCounters
	counters.CB = uint32(unsafe.Sizeof(counters))
	r, _, _ := getMem.Call(h, uintptr(unsafe.Pointer(&counters)), uintptr(counters.CB))
	if r == 0 {
		return 0
	}
	return float64(counters.WorkingSetSize) / (1024 * 1024)
}

func processCPUTime(pid uint32) uint64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProc := kernel32.NewProc("OpenProcess")
	getTimes := kernel32.NewProc("GetProcessTimes")
	closeHandle := kernel32.NewProc("CloseHandle")

	h, _, _ := openProc.Call(processQueryLimited, 0, uintptr(pid))
	if h == 0 {
		return 0
	}
	defer closeHandle.Call(h)

	var create, exit, kernel, user syscall.Filetime
	r, _, _ := getTimes.Call(
		h,
		uintptr(unsafe.Pointer(&create)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return 0
	}
	return filetimeToUint64(kernel) + filetimeToUint64(user)
}

func sampleProcesses(sysDelta uint64) []ProcessSnapshot {
	psapi := syscall.NewLazyDLL("psapi.dll")
	enumProc := psapi.NewProc("EnumProcesses")
	const maxPIDs = 4096
	pids := make([]uint32, maxPIDs)
	var needed uint32
	r, _, _ := enumProc.Call(
		uintptr(unsafe.Pointer(&pids[0])),
		uintptr(len(pids)*4),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return nil
	}
	count := int(needed / 4)
	if count > len(pids) {
		count = len(pids)
	}

	now := time.Now()
	procSampleMu.Lock()
	defer procSampleMu.Unlock()

	elapsed := now.Sub(lastProcSample).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	if lastProcCPU == nil {
		lastProcCPU = make(map[uint32]uint64)
	}

	rows := make([]ProcessSnapshot, 0, 64)
	for i := 0; i < count; i++ {
		pid := pids[i]
		if pid == 0 {
			continue
		}
		mem := processMemoryMB(pid)
		if mem < 0.5 {
			continue
		}
		cpuTime := processCPUTime(pid)
		cpuPct := 0.0
		if prev, ok := lastProcCPU[pid]; ok && lastProcSample.Unix() > 0 && sysDelta > 0 {
			delta := float64(cpuTime - prev)
			cpuPct = 100 * delta / float64(sysDelta)
			if cpuPct < 0 {
				cpuPct = 0
			}
		}
		lastProcCPU[pid] = cpuTime
		rows = append(rows, ProcessSnapshot{
			Name:  processImageName(pid),
			PID:   pid,
			CPU:   cpuPct,
			MemMB: mem,
		})
	}
	lastProcSample = now

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CPU == rows[j].CPU {
			return rows[i].MemMB > rows[j].MemMB
		}
		return rows[i].CPU > rows[j].CPU
	})
	if len(rows) > 40 {
		rows = rows[:40]
	}
	return rows
}

func logicalCPUs() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetSystemInfo")
	var si struct {
		wProcessorArchitecture      uint16
		wReserved                   uint16
		dwPageSize                  uint32
		lpMinimumApplicationAddress uintptr
		lpMaximumApplicationAddress uintptr
		dwActiveProcessorMask       uintptr
		dwNumberOfProcessors        uint32
		dwProcessorType             uint32
		dwAllocationGranularity     uint32
		wProcessorLevel             uint16
		wProcessorRevision          uint16
	}
	proc.Call(uintptr(unsafe.Pointer(&si)))
	if si.dwNumberOfProcessors == 0 {
		return 1
	}
	return int(si.dwNumberOfProcessors)
}
