//go:build windows

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows SDK: PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE
const procThreadAttributePseudoConsole = 0x00020016

func startHostShell(cols, rows int) (*hostShell, error) {
	cols, rows = clampTermSize(cols, rows)

	var inR, inW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, fmt.Errorf("conpty input pipe: %w", err)
	}
	var outR, outW windows.Handle
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, fmt.Errorf("conpty output pipe: %w", err)
	}

	var hPC windows.Handle
	size := windows.Coord{X: int16(cols), Y: int16(rows)}
	if err := windows.CreatePseudoConsole(size, inR, outW, 0, &hPC); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		windows.CloseHandle(outW)
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	_ = windows.CloseHandle(inR)
	_ = windows.CloseHandle(outW)

	attr, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, fmt.Errorf("attribute list: %w", err)
	}
	if err := attr.Update(procThreadAttributePseudoConsole, unsafe.Pointer(&hPC), unsafe.Sizeof(hPC)); err != nil {
		attr.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, fmt.Errorf("attribute update: %w", err)
	}

	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	}
	cmdLine, err := windows.UTF16PtrFromString(`"` + comspec + `"`)
	if err != nil {
		attr.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}

	dir := os.Getenv("USERPROFILE")
	if dir == "" {
		dir = `C:\`
	}
	dirPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		attr.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}

	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	si.ProcThreadAttributeList = attr.List()

	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	err = windows.CreateProcess(
		nil,
		cmdLine,
		nil,
		nil,
		false,
		flags,
		nil,
		dirPtr,
		&si.StartupInfo,
		&pi,
	)
	attr.Delete()
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)

	stdin := os.NewFile(uintptr(inW), "conpty-in")
	stdout := os.NewFile(uintptr(outR), "conpty-out")
	proc := pi.Process
	pc := hPC

	return &hostShell{
		stdin:  stdin,
		stdout: stdout,
		resize: func(c, r int) error {
			c, r = clampTermSize(c, r)
			return windows.ResizePseudoConsole(pc, windows.Coord{X: int16(c), Y: int16(r)})
		},
		kill: func() {
			windows.ClosePseudoConsole(pc)
			_ = stdin.Close()
			_ = stdout.Close()
			wait, werr := windows.WaitForSingleObject(proc, 3000)
			if werr != nil || wait != windows.WAIT_OBJECT_0 {
				_ = windows.TerminateProcess(proc, 1)
				_, _ = windows.WaitForSingleObject(proc, 1000)
			}
			_ = windows.CloseHandle(proc)
		},
	}, nil
}

func clampTermSize(cols, rows int) (int, int) {
	if cols < 20 {
		cols = 80
	}
	if cols > 400 {
		cols = 400
	}
	if rows < 5 {
		rows = 24
	}
	if rows > 200 {
		rows = 200
	}
	return cols, rows
}
