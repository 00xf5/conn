package agent

// Status is a snapshot for UI (tray, logs).
type Status struct {
	State   string // offline, online, streaming
	Session string
	Server  string
	Host    string
}

func (a *Agent) setState(state, session string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = state
	switch session {
	case "":
	case "-":
		a.activeSess = ""
	default:
		a.activeSess = session
	}
}

func (a *Agent) Snapshot() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := Status{
		State:   a.state,
		Session: a.activeSess,
		Server:  a.cfg.ServerURL,
		Host:    a.cfg.Hostname,
	}
	if s.State == "" {
		s.State = "offline"
	}
	return s
}
