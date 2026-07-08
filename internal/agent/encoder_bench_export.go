//go:build windows

package agent

import (
	"fmt"
	"os"
	"syscall"
)

func CollectAllFrames(data []byte) int {
	return CountTSFrames(data)
}

func CountTSFrames(data []byte) int {
	return countTSVideoFrames(data)
}

func CollectAllFramesIncremental(data []byte, chunk int) int {
	f := &ffmpegEncoder{}
	total := 0
	for i := 0; i < len(data); i += chunk {
		end := i + chunk
		if end > len(data) {
			end = len(data)
		}
		f.stash = append(f.stash, data[i:end]...)
		if len(f.stash) > 4096 {
			f.readDone = true
			for _, fr := range f.collectParsedFramesLocked() {
				if len(fr.Data) > 0 {
					total++
				}
			}
			f.readDone = false
		}
	}
	f.readDone = true
	for _, fr := range f.collectParsedFramesLocked() {
		if len(fr.Data) > 0 {
			total++
		}
	}
	return total
}

func EncoderFramesOut(enc videoEncoder) int64 {
	if fe, ok := enc.(*ffmpegEncoder); ok {
		return fe.FramesOut()
	}
	return 0
}

func EncoderBytesIn(enc videoEncoder) int64 {
	if fe, ok := enc.(*ffmpegEncoder); ok {
		return fe.BytesIn()
	}
	return 0
}

// HiddenProcAttr matches prepareHiddenCmd for bench diagnostics.
func HiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
}

// OpenVideoEncoderBench opens an encoder for diagnostics (experimental/encoder-bench only).
func OpenVideoEncoderBench(cfg Config, capture string, codec EncoderCodec) (videoEncoder, error) {
	if capture == "gdigrab" {
		return newGdiGrabFFmpegEncoderBench(cfg, codec)
	}
	return newDXGIFFmpegEncoder(cfg, codec)
}

// InitDevFFmpeg enables system ffmpeg lookup for bench tools.
func InitDevFFmpeg() {
	_ = os.Setenv("CONNECT_ALLOW_SYSTEM_FFMPEG", "1")
}

// DebugPopAccessUnits logs the first n pops (bench only).
func DebugPopAccessUnits(data []byte, max int) {
	f := &ffmpegEncoder{stash: append([]byte(nil), data...), readDone: true}
	for i := 0; i < max; i++ {
		if i == 4 && len(f.stash) > 0 {
			hdrSkip := 4
			if len(f.stash) > 3 && f.stash[2] == 0 {
				hdrSkip = 5
			}
			n := findStartCode(f.stash, hdrSkip)
			fmt.Printf("  pre[4] stash=%d type=%d next=%d head=% x\n", len(f.stash), nalTypeAt(f.stash, 0), n, f.stash[:min(20, len(f.stash))])
		}
		au, ok, key := f.popAccessUnit()
		fmt.Printf("  pop[%d] ok=%v key=%v auLen=%d stashLeft=%d\n", i, ok, key, len(au.data), len(f.stash))
		if !ok {
			break
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func CountAccessUnits(data []byte) int {
	f := &ffmpegEncoder{stash: append([]byte(nil), data...), readDone: true}
	n := 0
	for i := 0; i < 10000; i++ {
		au, ok, _ := f.popAccessUnit()
		if !ok {
			break
		}
		if len(au.data) > 0 {
			n++
		}
	}
	return n
}
