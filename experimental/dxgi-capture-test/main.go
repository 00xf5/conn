//go:build windows && cgo

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"connect/internal/captureenc"
)

func main() {
	cap, err := captureenc.NewCaptureOnly(captureenc.CaptureConfig{FPS: 20, Width: 854, Height: 480})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL capture: %v\n", err)
		os.Exit(1)
	}
	defer cap.Close()

	got := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && got < 3 {
		f, err := cap.ReadNV12()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL read: %v\n", err)
			os.Exit(2)
		}
		if len(f.Data) == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		got++
		log.Printf("nv12 frame %d: %dx%d pitch=%d bytes=%d", got, f.Width, f.Height, f.Pitch, len(f.Data))
	}
	if got == 0 {
		fmt.Fprintln(os.Stderr, "FAIL no nv12 frames")
		os.Exit(3)
	}
	log.Printf("OK dxgi capture %d frames", got)
}
