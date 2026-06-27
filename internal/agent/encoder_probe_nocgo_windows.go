//go:build windows && !cgo

package agent

import "log"

func resolveEncoderCodec(cfg Config) EncoderCodec {
	_ = cfg
	log.Printf("agent: CGO disabled; using %s", CodecX264)
	return CodecX264
}
