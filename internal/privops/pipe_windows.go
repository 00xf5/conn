//go:build windows

package privops

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// ServePipe listens for privileged op requests from the interactive agent.
func ServePipe(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		conn, err := acceptOne()
		if err != nil {
			select {
			case <-stop:
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		go handleConn(conn)
	}
}

func acceptOne() (net.Conn, error) {
	path, err := windows.UTF16PtrFromString(PipeName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateNamedPipe(
		path,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1,
		4096,
		4096,
		0,
		nil,
	)
	if err != nil {
		return nil, err
	}
	err = windows.ConnectNamedPipe(h, nil)
	if err == windows.ERROR_PIPE_CONNECTED {
		err = nil
	}
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	f := os.NewFile(uintptr(h), "priv-pipe")
	conn, err := net.FileConn(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Dial connects to the SYSTEM priv pipe (from the interactive agent).
func Dial(timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		conn, err := dialOnce()
		if err == nil {
			return conn, nil
		}
		last = err
		time.Sleep(200 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("priv pipe unavailable (is ConnectAgent service running?)")
	}
	return nil, last
}

func dialOnce() (net.Conn, error) {
	path, err := windows.UTF16PtrFromString(PipeName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), "priv-pipe-client")
	conn, err := net.FileConn(f)
	_ = f.Close()
	return conn, err
}
