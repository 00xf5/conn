//go:build windows

package agent

import (
	"sync"
	"syscall"
	"unsafe"
)

type hostMetrics struct {
	CPU         float64
	MemUsedGB   float64
	MemTotalGB  float64
	MemPct      float64
	DiskFreeGB  float64
	DiskTotalGB float64
	UptimeSec   uint64
	Processes   []ProcessSnapshot
	CPUCores    int
}

var (
	cpuMu          sync.Mutex
	lastIdle       uint64
	lastKernel     uint64
	lastUser       uint64
	cpuInitialized bool
)

type memoryStatusEx struct {
	Length                uint32
	MemoryLoad            uint32
	TotalPhys             uint64
	AvailPhys             uint64
	TotalPageFile         uint64
	AvailPageFile         uint64
	TotalVirtual          uint64
	AvailVirtual          uint64
	AvailExtendedVirtual  uint64
}

func filetimeToUint64(ft syscall.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

func sampleCPU() float64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetSystemTimes")
	var idleFT, kernelFT, userFT syscall.Filetime
	r, _, _ := proc.Call(
		uintptr(unsafe.Pointer(&idleFT)),
		uintptr(unsafe.Pointer(&kernelFT)),
		uintptr(unsafe.Pointer(&userFT)),
	)
	if r == 0 {
		return 0
	}
	idle := filetimeToUint64(idleFT)
	kernel := filetimeToUint64(kernelFT)
	user := filetimeToUint64(userFT)

	cpuMu.Lock()
	defer cpuMu.Unlock()
	if !cpuInitialized {
		lastIdle, lastKernel, lastUser = idle, kernel, user
		cpuInitialized = true
		return 0
	}
	idleDelta := idle - lastIdle
	totalDelta := (kernel - lastKernel) + (user - lastUser) + idleDelta
	lastIdle, lastKernel, lastUser = idle, kernel, user
	if totalDelta == 0 {
		return 0
	}
	return 100 * (1 - float64(idleDelta)/float64(totalDelta))
}

func systemTimeDelta() uint64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetSystemTimes")
	var idleFT, kernelFT, userFT syscall.Filetime
	r, _, _ := proc.Call(
		uintptr(unsafe.Pointer(&idleFT)),
		uintptr(unsafe.Pointer(&kernelFT)),
		uintptr(unsafe.Pointer(&userFT)),
	)
	if r == 0 {
		return 0
	}
	idle := filetimeToUint64(idleFT)
	kernel := filetimeToUint64(kernelFT)
	user := filetimeToUint64(userFT)
	cpuMu.Lock()
	defer cpuMu.Unlock()
	if !cpuInitialized {
		return 0
	}
	idleDelta := idle - lastIdle
	return (kernel - lastKernel) + (user - lastUser) + idleDelta
}

func sampleMemory() (usedGB, totalGB, pct float64) {
	usedGB, totalGB, pct, _, _ = sampleMemoryFull()
	return usedGB, totalGB, pct
}

func sampleMemoryFull() (usedGB, totalGB, pct, pageTotGB, pageAvailGB float64) {
	var st memoryStatusEx
	st.Length = uint32(unsafe.Sizeof(st))
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GlobalMemoryStatusEx")
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return 0, 0, 0, 0, 0
	}
	const gib = 1024 * 1024 * 1024
	totalGB = float64(st.TotalPhys) / gib
	usedGB = float64(st.TotalPhys-st.AvailPhys) / gib
	pct = float64(st.MemoryLoad)
	pageTotGB = float64(st.TotalPageFile) / gib
	pageAvailGB = float64(st.AvailPageFile) / gib
	return usedGB, totalGB, pct, pageTotGB, pageAvailGB
}

func sampleDiskC() (freeGB, totalGB float64) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")
	path, _ := syscall.UTF16PtrFromString(`C:\`)
	var free, total uint64
	r, _, _ := proc.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&free)),
		uintptr(unsafe.Pointer(&total)),
		0,
	)
	if r == 0 {
		return 0, 0
	}
	return float64(free) / (1024 * 1024 * 1024), float64(total) / (1024 * 1024 * 1024)
}

func sampleUptime() uint64 {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetTickCount64")
	ms, _, _ := proc.Call()
	return uint64(ms) / 1000
}

func sampleHostMetrics() hostMetrics {
	used, total, pct := sampleMemory()
	free, diskTotal := sampleDiskC()
	return hostMetrics{
		CPU:         sampleCPU(),
		MemUsedGB:   used,
		MemTotalGB:  total,
		MemPct:      pct,
		DiskFreeGB:  free,
		DiskTotalGB: diskTotal,
		UptimeSec:   sampleUptime(),
		Processes:   sampleProcesses(systemTimeDelta()),
		CPUCores:    logicalCPUs(),
	}
}
