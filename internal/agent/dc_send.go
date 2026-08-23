package agent

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	// Raw file bytes per control chunk. Base64 expands ~4/3, then JSON wraps the
	// payload; keep well under the common 64 KiB WebRTC data-channel message cap.
	dcFileChunkBytes = 24 << 10
	// Pause sends while the SCTP outbound buffer is above this (bytes).
	dcSendBufHigh = 256 << 10
	// Max single-file pull/push over the control data channel.
	dcMaxFileBytes = 1 << 30 // 1 GiB
)

func dcWaitSend(dc *webrtc.DataChannel) error {
	if dc == nil {
		return errString("no data channel")
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if dc.ReadyState() != webrtc.DataChannelStateOpen {
			return errString("data channel closed")
		}
		if dc.BufferedAmount() <= uint64(dcSendBufHigh) {
			return nil
		}
		if time.Now().After(deadline) {
			return errString("send buffer timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func dcSendText(dc *webrtc.DataChannel, text string) error {
	if err := dcWaitSend(dc); err != nil {
		return err
	}
	return dc.SendText(text)
}

// dcStreamFileToDC reads path in small chunks and sends base64 control_result parts.
// Does not load the whole file into memory (needed for multi‑hundred‑MB / 1 GB pulls).
func dcStreamFileToDC(dc *webrtc.DataChannel, action, name, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errString("path is a directory")
	}
	size := info.Size()
	if size > dcMaxFileBytes {
		return errString("file too large (max 1 GB)")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sendPart := func(idx int, chunk []byte, done bool) error {
		part := map[string]any{
			"type":   "control_result",
			"action": action,
			"name":   name,
			"idx":    idx,
			"done":   done,
			"size":   size,
			"data":   base64.StdEncoding.EncodeToString(chunk),
		}
		// Path only on first part — keeps later messages smaller.
		if idx == 0 && path != "" {
			part["path"] = path
		}
		raw, err := json.Marshal(part)
		if err != nil {
			return err
		}
		return dcSendText(dc, string(raw))
	}

	if size == 0 {
		return sendPart(0, nil, true)
	}

	buf := make([]byte, dcFileChunkBytes)
	var sent int64
	idx := 0
	for sent < size {
		want := len(buf)
		if left := size - sent; int64(want) > left {
			want = int(left)
		}
		n, err := io.ReadFull(f, buf[:want])
		if n > 0 {
			sent += int64(n)
			if err := sendPart(idx, buf[:n], sent >= size); err != nil {
				return err
			}
			idx++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if sent < size {
				return errString("file read truncated")
			}
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
