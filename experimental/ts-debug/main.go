//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"connect/internal/agent"

	"github.com/asticode/go-astits"
)

func main() {
	path := os.Getenv("TEMP") + `\bench.ts`
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if _, err := os.Stat(path); err != nil {
		capture(path)
	}
	data, _ := os.ReadFile(path)
	fmt.Printf("file bytes=%d\n", len(data))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dmx := astits.NewDemuxer(ctx, bytes.NewReader(data))

	pesCount := 0
	videoPES := 0
	withSC := 0
	totalPESBytes := 0
	var concat []byte
	for {
		d, err := dmx.NextData()
		if err != nil {
			fmt.Println("demux err:", err)
			break
		}
		if d.PAT != nil {
			continue
		}
		if d.PMT != nil {
			continue
		}
		if d.PES == nil {
			continue
		}
		pesCount++
		p := d.PES.Data
		totalPESBytes += len(p)
		concat = append(concat, p...)
		if d.PES.Header != nil && d.PES.Header.StreamID >= 0xE0 && d.PES.Header.StreamID <= 0xEF {
			videoPES++
		}
		if hasStartCode(p) {
			withSC++
		}
		if pesCount <= 3 {
			hdr := 0
			if d.PES.Header != nil {
				hdr = int(d.PES.Header.StreamID)
			}
			fmt.Printf("pes[%d] stream=0x%02x len=%d head=% x\n", pesCount, hdr, len(p), head(p, 24))
		}
	}
	fmt.Printf("pes=%d videoPES=%d withStartCode=%d pesBytes=%d concatLen=%d\n", pesCount, videoPES, withSC, totalPESBytes, len(concat))
	fmt.Printf("concat CountAccessUnits=%d CountTSFrames=%d\n", agent.CountAccessUnits(concat), agent.CountTSFrames(data))

	// replicate demuxTSAll inline
	var stash []byte
	dmx2 := astits.NewDemuxer(context.Background(), bytes.NewReader(data))
	for {
		pkt, err := dmx2.NextData()
		if err != nil {
			break
		}
		if pkt.PES == nil || len(pkt.PES.Data) == 0 {
			continue
		}
		stash = append(stash, pkt.PES.Data...)
	}
	fmt.Printf("inline stash=%d CU=%d match=%v\n", len(stash), agent.CountAccessUnits(stash), bytes.Equal(stash, concat))
}

func capture(path string) {
	ff := `C:\Users\shiver\AppData\Local\Microsoft\WinGet\Packages\Gyan.FFmpeg_Microsoft.Winget.Source_8wekyb3d8bbwe\ffmpeg-8.1.1-full_build\bin\ffmpeg.exe`
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "gdigrab", "-framerate", "20", "-draw_mouse", "1", "-t", "15",
		"-i", "desktop", "-an",
		"-vf", "scale=854:480:flags=fast_bilinear,fps=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
		"-profile:v", "baseline", "-pix_fmt", "yuv420p",
		"-b:v", "2000k", "-r", "20", "-g", "40", "-bf", "0",
		"-f", "mpegts", "-mpegts_flags", "+resend_headers", "-flush_packets", "1", path,
	}
	cmd := exec.Command(ff, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("capture:", err, string(out))
	}
}

func hasStartCode(b []byte) bool {
	for i := 0; i+3 < len(b) && i < 8; i++ {
		if b[i] == 0 && b[i+1] == 0 && (b[i+2] == 1 || (i+3 < len(b) && b[i+2] == 0 && b[i+3] == 1)) {
			return true
		}
	}
	return false
}

func head(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}
