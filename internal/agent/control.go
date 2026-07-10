package agent

import (
	"encoding/json"
	"log"
	"time"

	"github.com/pion/webrtc/v4"
)

type controlMsg struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	Text     string `json:"text,omitempty"`
	URL      string `json:"url,omitempty"`
	Cmd      string `json:"cmd,omitempty"`
	Name     string `json:"name,omitempty"`
	Data     string `json:"data,omitempty"`
	Idx      int    `json:"idx,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Dest     string `json:"dest,omitempty"`
	BitrateK int    `json:"bitrateK,omitempty"`
	Monitor  int    `json:"monitor,omitempty"`
}

func (a *Agent) handleControl(data []byte, dc *webrtc.DataChannel) {
	var msg controlMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	log.Printf("agent: control action %s", msg.Action)

	var result map[string]any
	var err error

	switch msg.Action {
	case "ctrl_alt_del":
		err = controlCtrlAltDel()
	case "lock":
		err = controlLock()
	case "shutdown":
		err = controlShutdown(false)
	case "reboot":
		err = controlShutdown(true)
	case "clipboard":
		err = controlClipboard(msg.Text)
	case "open_url":
		err = controlOpenURL(msg.URL)
	case "run":
		err = controlRun(msg.Cmd)
	case "win_d":
		err = controlWinD()
	case "set_bitrate":
		err = a.controlSetBitrate(msg.BitrateK)
	case "file_begin":
		err = controlFileBegin(msg.Name, msg.Size)
	case "file_chunk":
		err = controlFileChunk(msg.Idx, msg.Data)
	case "file_end":
		path, e := controlFileEnd()
		err = e
		if err == nil {
			result = map[string]any{"path": path}
		}
	case "list_files":
		files, e := controlListDownloads()
		err = e
		if err == nil {
			result = map[string]any{"files": files}
		}
	case "download_file":
		err = a.controlSendFile(msg.Name, dc)
	default:
		err = errControlUnknown
	}

	a.sendControlResult(dc, msg.Action, result, err)
}

func (a *Agent) controlSetBitrate(kbps int) error {
	kbps = ProfileFromConfig(a.cfg).ClampBitrate(kbps)
	a.mu.Lock()
	a.cfg.BitrateK = kbps
	a.abrBitrateK = kbps
	a.abrHoldUntil = time.Now().Add(abrManualHold)
	a.abrLastAdjust = time.Now()
	enc := a.enc
	a.mu.Unlock()
	if enc != nil {
		return enc.SetBitrate(kbps)
	}
	return nil
}

func (a *Agent) sendControlResult(dc *webrtc.DataChannel, action string, result map[string]any, err error) {
	if dc == nil {
		return
	}
	payload := map[string]any{
		"type":   "control_result",
		"action": action,
		"ok":     err == nil,
	}
	if result != nil {
		for k, v := range result {
			payload[k] = v
		}
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	raw, _ := json.Marshal(payload)
	_ = dc.SendText(string(raw))
}

var (
	errControlUnknown = errString("unknown control action")
	errControlInvalid = errString("invalid control request")
)

type errString string

func (e errString) Error() string { return string(e) }
