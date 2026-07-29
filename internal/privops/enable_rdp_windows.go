//go:build windows

package privops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Handle runs a privileged op. Intended for LocalSystem only.
func Handle(req Request) Response {
	switch strings.TrimSpace(req.Op) {
	case OpEnableRDP:
		return enableRDP(req)
	default:
		return Response{OK: false, Error: "unknown privileged op"}
	}
}

func enableRDP(req Request) Response {
	user := strings.TrimSpace(req.Username)
	pass := req.Password

	if err := setRDPEnabled(true); err != nil {
		return Response{OK: false, Error: "enable RDP: " + err.Error()}
	}
	if err := enableRDPFirewall(); err != nil {
		return Response{OK: false, Error: "firewall: " + err.Error()}
	}

	detail := "RDP enabled; firewall Remote Desktop group on"
	if user != "" {
		if pass == "" {
			return Response{OK: false, Error: "password required when creating/updating a user"}
		}
		if err := ensureRDPUser(user, pass); err != nil {
			return Response{OK: false, Error: err.Error(), Username: user, Detail: detail}
		}
		detail += "; user ready for RDP logon"
		return Response{OK: true, Username: user, Detail: detail}
	}
	return Response{OK: true, Detail: detail + " (use an existing Windows account to sign in)"}
}

func setRDPEnabled(on bool) error {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Terminal Server`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()
	var deny uint32 = 1
	if on {
		deny = 0
	}
	return k.SetDWordValue("fDenyTSConnections", deny)
}

func enableRDPFirewall() error {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "set", "rule", "group=Remote Desktop", "new", "enable=Yes")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureRDPUser(user, pass string) error {
	if strings.ContainsAny(user, `/\`) || strings.Contains(user, " ") {
		return fmt.Errorf("username must be a simple local name (no domain path or spaces)")
	}
	if len(user) < 2 || len(user) > 20 {
		return fmt.Errorf("username length invalid")
	}
	if len(pass) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	add := exec.Command("net", "user", user, pass, "/add", "/y")
	if out, err := add.CombinedOutput(); err != nil {
		msg := strings.ToLower(string(out))
		if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "1378") {
			return fmt.Errorf("net user /add: %s", strings.TrimSpace(string(out)))
		}
		set := exec.Command("net", "user", user, pass)
		if out2, err2 := set.CombinedOutput(); err2 != nil {
			return fmt.Errorf("net user set password: %s", strings.TrimSpace(string(out2)))
		}
	}

	_ = exec.Command("net", "user", user, "/active:yes").Run()

	for _, group := range []string{"Remote Desktop Users", "Administrators"} {
		cmd := exec.Command("net", "localgroup", group, user, "/add")
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.ToLower(string(out))
			if strings.Contains(msg, "already") || strings.Contains(msg, "1378") {
				continue
			}
			if group == "Remote Desktop Users" {
				continue
			}
			return fmt.Errorf("localgroup %s: %s", group, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultTimeout))
	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	var req Request
	if json.Unmarshal(line, &req) != nil {
		writeResp(conn, Response{OK: false, Error: "bad request"})
		return
	}
	writeResp(conn, Handle(req))
}

func writeResp(conn net.Conn, resp Response) {
	raw, _ := json.Marshal(resp)
	raw = append(raw, '\n')
	_, _ = conn.Write(raw)
}

// Call sends a privileged request to the LocalSystem service pipe.
func Call(req Request) Response {
	conn, err := Dial(5 * time.Second)
	if err != nil {
		return Response{OK: false, Error: err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultTimeout))
	raw, err := json.Marshal(req)
	if err != nil {
		return Response{OK: false, Error: "encode failed"}
	}
	raw = append(raw, '\n')
	if _, err := conn.Write(raw); err != nil {
		return Response{OK: false, Error: "write failed: " + err.Error()}
	}
	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Response{OK: false, Error: "read failed: " + err.Error()}
	}
	var resp Response
	if json.Unmarshal(line, &resp) != nil {
		return Response{OK: false, Error: "bad response"}
	}
	return resp
}
