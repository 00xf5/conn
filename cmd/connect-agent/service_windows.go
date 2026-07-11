//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "ConnectAgent"
	serviceDisplay = "BlueConnect Host Agent"
	serviceDesc    = "Connect remote access host agent supervisor. Keeps the interactive capture agent running across reboot, lock, and crash."
)

func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

func runServiceMode() error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		// Still run even if event log source is missing.
		elog = nil
	}
	defer func() {
		if elog != nil {
			_ = elog.Close()
		}
	}()
	return svc.Run(serviceName, &connectService{elog: elog})
}

type connectService struct {
	elog *eventlog.Log
}

func (s *connectService) logInfo(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if s.elog != nil {
		_ = s.elog.Info(1, msg)
	}
}

func (s *connectService) logError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if s.elog != nil {
		_ = s.elog.Error(1, msg)
	}
}

func (s *connectService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.supervise(stop)
	}()
	changes <- svc.Status{State: svc.Running, Accepts: accepts}
	s.logInfo("Connect Agent service running")

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			close(stop)
			<-done
			s.logInfo("Connect Agent service stopped")
			return false, 0
		default:
			// ignore
		}
	}
	return false, 0
}

func (s *connectService) supervise(stop <-chan struct{}) {
	exe, err := os.Executable()
	if err != nil {
		s.logError("executable path: %v", err)
		return
	}
	exe, _ = filepath.Abs(exe)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		if interactiveAgentRunning() {
			continue
		}

		sessionID := windows.WTSGetActiveConsoleSessionId()
		if sessionID == 0xFFFFFFFF {
			continue // no interactive session yet
		}

		if err := launchAgentInSession(sessionID, exe); err != nil {
			s.logError("launch session %d: %v", sessionID, err)
			continue
		}
		s.logInfo("launched interactive agent in session %d", sessionID)
	}
}

func serviceInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	_ = s.Close()
	return true
}

func installService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect (need Administrator): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		_ = s.Close()
		// Update bin path / ensure recovery, then start.
		return configureAndStartExisting(m, exe)
	}

	s, err = m.CreateService(
		serviceName,
		exe,
		mgr.Config{
			DisplayName:      serviceDisplay,
			Description:      serviceDesc,
			StartType:        mgr.StartAutomatic,
			ServiceStartName: "", // LocalSystem
		},
		"-service",
	)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	defer s.Close()

	_ = eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err := setRecovery(s); err != nil {
		// Non-fatal: service still works without recovery actions.
		fmt.Fprintf(os.Stderr, "connect-agent: recovery config: %v\n", err)
	}
	if err := s.Start("-service"); err != nil {
		// Start may fail if already starting; try query.
		return fmt.Errorf("StartService: %w", err)
	}
	return nil
}

func configureAndStartExisting(m *mgr.Mgr, exe string) error {
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	cfg, err := s.Config()
	if err == nil {
		cfg.BinaryPathName = `"` + exe + `" -service`
		cfg.StartType = mgr.StartAutomatic
		cfg.DisplayName = serviceDisplay
		cfg.Description = serviceDesc
		_ = s.UpdateConfig(cfg)
	}
	_ = setRecovery(s)

	// Always recycle so an upgrade picks up the new binary (seamless reinstall).
	status, qerr := s.Query()
	if qerr == nil && status.State != svc.Stopped && status.State != svc.StopPending {
		_, _ = s.Control(svc.Stop)
	}
	for i := 0; i < 50; i++ {
		st, err := s.Query()
		if err != nil || st.State == svc.Stopped {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return s.Start("-service")
}

func setRecovery(s *mgr.Service) error {
	actions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
	}
	return s.SetRecoveryActions(actions, 86400) // reset fail count after 1 day
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect (need Administrator): %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service not installed")
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	// Wait briefly for stop.
	for i := 0; i < 30; i++ {
		st, err := s.Query()
		if err != nil || st.State == svc.Stopped {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := s.Delete(); err != nil {
		return err
	}
	_ = eventlog.Remove(serviceName)
	return nil
}
