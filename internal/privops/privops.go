package privops

import "time"

const PipeName = `\\.\pipe\ConnectAgentPriv`

type Request struct {
	Op       string `json:"op"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type Response struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Username string `json:"username,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

const DefaultTimeout = 45 * time.Second

const OpEnableRDP = "enable_rdp"
