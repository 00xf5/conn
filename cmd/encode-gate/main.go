//go:build windows

package main

import (
	"fmt"
	"os"
	"time"

	"connect/internal/agent"
	"connect/internal/captureenc"
)

func main() {
	cfg := agent.NormalizeConfig(agent.Config{
		Width: 854, Height: 480, FPS: 20, BitrateK: 2000,
	})
	prof := agent.ProfileFromConfig(cfg)

	pipe, err := captureenc.OpenHostPipeline(captureenc.HostConfig{
		Width: prof.Width, Height: prof.Height, FPS: prof.FPS, BitrateK: prof.BitrateK,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode-gate: open failed: %v\n", err)
		os.Exit(1)
	}
	defer pipe.Close()

	fmt.Printf("encode-gate: encoder=%s size=%dx%d duration=15s\n",
		pipe.EncoderName(), prof.Width, prof.Height)
	cw, ch := pipe.CaptureSize()
	if cw > 0 && ch > 0 {
		fmt.Printf("encode-gate: capture=%dx%d\n", cw, ch)
	}

	deadline := time.Now().Add(15 * time.Second)
	const gateSecs = 15.0
	const gateMinFPS = 12.0
	var frames, keys int
	var minP, maxP int

	for time.Now().Before(deadline) {
		au, err := pipe.ReadAccessUnit()
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode-gate: read: %v\n", err)
			os.Exit(1)
		}
		if len(au.Data) == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		frames++
		if au.KeyFrame {
			keys++
			fmt.Printf("  keyframe bytes=%d\n", len(au.Data))
		} else {
			if minP == 0 || len(au.Data) < minP {
				minP = len(au.Data)
			}
			if len(au.Data) > maxP {
				maxP = len(au.Data)
			}
		}
	}

	fps := float64(frames) / gateSecs
	fmt.Printf("encode-gate: frames=%d keys=%d fps=%.1f p_min=%d p_max=%d\n", frames, keys, fps, minP, maxP)

	minFrames := int(gateMinFPS * gateSecs)
	if frames < minFrames {
		fmt.Fprintf(os.Stderr, "encode-gate: FAIL frame count too low (%d < %d)\n", frames, minFrames)
		os.Exit(1)
	}
	if fps < gateMinFPS {
		fmt.Fprintf(os.Stderr, "encode-gate: FAIL fps too low (%.1f < %.1f)\n", fps, gateMinFPS)
		os.Exit(1)
	}
	if keys < 1 {
		fmt.Fprintln(os.Stderr, "encode-gate: FAIL no keyframe")
		os.Exit(1)
	}
	if minP > 0 && minP < captureenc.MinDeltaBytes {
		fmt.Fprintf(os.Stderr, "encode-gate: FAIL tiny P-frame (%d bytes)\n", minP)
		os.Exit(1)
	}
	fmt.Println("encode-gate: PASS")
}
