//go:build !windows

package agent

func resolveEncoderCodec(cfg Config) EncoderCodec {
	_ = cfg
	return CodecX264
}
