//go:build windows

package agent

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/asticode/go-astits"
)

type tsVideoDemux struct {
	ctx    context.Context
	cancel context.CancelFunc
	pw     *io.PipeWriter
	wg     sync.WaitGroup

	mu      sync.Mutex
	pending []videoFrame
	parse   ffmpegEncoder
}

func newTSVideoDemux() *tsVideoDemux {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	d := &tsVideoDemux{ctx: ctx, cancel: cancel, pw: pw}
	d.wg.Add(1)
	go d.pump(pr)
	return d
}

func (d *tsVideoDemux) Write(p []byte) (int, error) {
	return d.pw.Write(p)
}

func (d *tsVideoDemux) Drain() []videoFrame {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.pending
	d.pending = nil
	return out
}

func (d *tsVideoDemux) finish() []videoFrame {
	_ = d.pw.Close()
	d.cancel()
	d.wg.Wait()
	d.mu.Lock()
	d.parse.readDone = true
	frames := d.collectParsedLocked()
	d.mu.Unlock()
	return append(d.Drain(), frames...)
}

func (d *tsVideoDemux) Close() {
	_ = d.pw.Close()
	d.cancel()
	d.wg.Wait()
}

func (d *tsVideoDemux) collectParsedLocked() []videoFrame {
	var frames []videoFrame
	for {
		au, ok, key := d.parse.popAccessUnit()
		if !ok {
			break
		}
		fr := videoFrame{Data: au.data, KeyFrame: key}
		if !acceptVideoFrame(fr) {
			continue
		}
		frames = append(frames, fr)
	}
	return frames
}

func (d *tsVideoDemux) onPES(pes []byte) {
	if len(pes) == 0 {
		return
	}
	d.parse.stash = append(d.parse.stash, pes...)
	frames := d.collectParsedLocked()
	if len(frames) > 0 {
		d.pending = append(d.pending, frames...)
	}
}

func (d *tsVideoDemux) pump(pr *io.PipeReader) {
	defer d.wg.Done()
	defer pr.Close()
	dmx := astits.NewDemuxer(d.ctx, bufio.NewReader(pr))
	for {
		data, err := dmx.NextData()
		if err != nil {
			return
		}
		if data.PES == nil || len(data.PES.Data) == 0 {
			continue
		}
		d.mu.Lock()
		d.onPES(data.PES.Data)
		d.mu.Unlock()
	}
}

func demuxTSAll(data []byte) []videoFrame {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dmx := astits.NewDemuxer(ctx, bytes.NewReader(data))
	var parse ffmpegEncoder
	for {
		pkt, err := dmx.NextData()
		if err != nil {
			break
		}
		if pkt.PES == nil || len(pkt.PES.Data) == 0 {
			continue
		}
		parse.stash = append(parse.stash, pkt.PES.Data...)
	}
	parse.readDone = true
	var frames []videoFrame
	for {
		au, ok, key := parse.popAccessUnit()
		if !ok {
			break
		}
		if len(au.data) == 0 {
			continue
		}
		frames = append(frames, videoFrame{Data: au.data, KeyFrame: key})
	}
	return frames
}

func countTSVideoFrames(data []byte) int {
	n := 0
	for _, fr := range demuxTSAll(data) {
		if len(fr.Data) > 0 {
			n++
		}
	}
	return n
}

func countTSFramesFromReader(stdout io.Reader, deadline time.Time) int {
	demux := newTSVideoDemux()
	defer demux.Close()
	frames := 0
	for time.Now().Before(deadline) {
		chunk := make([]byte, 32768)
		n, readErr := stdout.Read(chunk)
		if n > 0 {
			_, _ = demux.Write(chunk[:n])
			for _, fr := range demux.Drain() {
				if len(fr.Data) > 0 {
					frames++
				}
			}
		}
		if readErr != nil {
			break
		}
	}
	for _, fr := range demux.finish() {
		if len(fr.Data) > 0 {
			frames++
		}
	}
	return frames
}
