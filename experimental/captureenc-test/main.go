//go:build windows && cgo && experimental

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"connect/internal/captureenc"
)

func main() {
	enc, err := captureenc.New(captureenc.Config{FPS: 30, BitrateK: 4000})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL init: %v\n", err)
		os.Exit(1)
	}
	defer enc.Close()
	log.Printf("encoder: %s", enc.EncoderName())

	got := 0
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && got < 5 {
		f, err := enc.ReadFrame()
		if err != nil {
			log.Printf("read error: %v", err)
			break
		}
		if len(f.Data) == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		got++
		log.Printf("frame %d: %d bytes key=%v", got, len(f.Data), f.KeyFrame)
	}
	if got == 0 {
		fmt.Fprintln(os.Stderr, "FAIL no frames")
		os.Exit(2)
	}
	log.Printf("OK %d frames from %s", got, enc.EncoderName())
}
