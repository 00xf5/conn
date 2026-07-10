//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modWtsapi32           = windows.NewLazySystemDLL("wtsapi32.dll")
	modUserenv            = windows.NewLazySystemDLL("userenv.dll")
	procWTSQueryUserToken = modWtsapi32.NewProc("WTSQueryUserToken")
	procCreateEnvBlock    = modUserenv.NewProc("CreateEnvironmentBlock")
	procDestroyEnvBlock   = modUserenv.NewProc("DestroyEnvironmentBlock")
)

func interactiveAgentRunning() bool {
	name, err := windows.UTF16PtrFromString(`Global\ConnectHostAgent`)
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}

func wtsQueryUserToken(sessionID uint32) (windows.Token, error) {
	var h windows.Handle
	r1, _, e1 := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&h)))
	if r1 == 0 {
		if e1 != nil && e1 != syscall.Errno(0) {
			return 0, e1
		}
		return 0, fmt.Errorf("WTSQueryUserToken failed")
	}
	return windows.Token(h), nil
}

func createEnvironmentBlock(token windows.Token) (*uint16, error) {
	var env *uint16
	r1, _, e1 := procCreateEnvBlock.Call(
		uintptr(unsafe.Pointer(&env)),
		uintptr(token),
		0,
	)
	if r1 == 0 {
		if e1 != nil && e1 != syscall.Errno(0) {
			return nil, e1
		}
		return nil, fmt.Errorf("CreateEnvironmentBlock failed")
	}
	return env, nil
}

func destroyEnvironmentBlock(env *uint16) {
	if env != nil {
		_, _, _ = procDestroyEnvBlock.Call(uintptr(unsafe.Pointer(env)))
	}
}

// launchAgentInSession starts connect-agent.exe inside the interactive user
// session (Session N). DXGI capture cannot run in Session 0.
func launchAgentInSession(sessionID uint32, agentExe string) error {
	userToken, err := wtsQueryUserToken(sessionID)
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	defer userToken.Close()

	var primary windows.Token
	err = windows.DuplicateTokenEx(
		userToken,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&primary,
	)
	if err != nil {
		return fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer primary.Close()

	env, err := createEnvironmentBlock(primary)
	if err != nil {
		return fmt.Errorf("env: %w", err)
	}
	defer destroyEnvironmentBlock(env)

	dir := filepath.Dir(agentExe)
	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return err
	}
	appName, err := windows.UTF16PtrFromString(agentExe)
	if err != nil {
		return err
	}
	cmdLine, err := windows.UTF16PtrFromString(`"` + agentExe + `"`)
	if err != nil {
		return err
	}
	cwd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = desktop
	si.Flags = windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_SHOWNA

	var pi windows.ProcessInformation
	err = windows.CreateProcessAsUser(
		primary,
		appName,
		cmdLine,
		nil,
		nil,
		false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NEW_PROCESS_GROUP,
		env,
		cwd,
		&si,
		&pi,
	)
	if err != nil {
		return fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}
