//go:build windows

package agent

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/asticode/go-astits"
)

func TestDemuxTSFrameCount(t *testing.T) {
	path := os.Getenv("TEMP") + `\bench.ts`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no bench.ts:", err)
	}
	concat := extractTSVideoES(data)
	au := CountAccessUnits(concat)
	ts := countTSVideoFrames(data)
	direct := len(demuxTSAll(data))
	var parse ffmpegEncoder
	parse.stash = append(parse.stash, concat...)
	parse.readDone = true
	incremental := 0
	for {
		au, ok, _ := parse.popAccessUnit()
		if !ok {
			break
		}
		if len(au.data) > 0 {
			incremental++
		}
	}
	f := &ffmpegEncoder{stash: append([]byte(nil), concat...), readDone: true}
	copied := 0
	for {
		au, ok, _ := f.popAccessUnit()
		if !ok {
			break
		}
		if len(au.data) > 0 {
			copied++
		}
	}
	t.Logf("concat=%d accessUnits=%d tsFrames=%d direct=%d incremental=%d copied=%d", len(concat), au, ts, direct, incremental, copied)
	if au < 200 {
		t.Fatalf("concat parse low: %d", au)
	}
	if ts != au {
		t.Fatalf("TS frames=%d accessUnits=%d", ts, au)
	}
}

func extractTSVideoES(data []byte) []byte {
	ctx, cancel := contextWithCancel()
	defer cancel()
	dmx := astits.NewDemuxer(ctx, bytes.NewReader(data))
	var concat []byte
	for {
		pkt, err := dmx.NextData()
		if err != nil {
			break
		}
		if pkt.PES == nil || len(pkt.PES.Data) == 0 {
			continue
		}
		concat = append(concat, pkt.PES.Data...)
	}
	return concat
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
