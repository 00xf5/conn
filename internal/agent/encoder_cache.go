package agent

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

type encoderCacheFile struct {
	Version   int     `json:"version"`
	Codec     string  `json:"codec"`
	Label     string  `json:"label"`
	ProbedFPS float64 `json:"probedFps"`
	ProbedAt  string  `json:"probedAt"`
}

func encoderCachePath() string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		return "encoder.json"
	}
	return filepath.Join(dir, "Connect", "encoder.json")
}

func loadCachedEncoderCodec() (EncoderCodec, bool) {
	b, err := os.ReadFile(encoderCachePath())
	if err != nil {
		return "", false
	}
	var f encoderCacheFile
	if err := json.Unmarshal(b, &f); err != nil {
		return "", false
	}
	if f.Version != encoderCacheVersion || f.Codec == "" {
		return "", false
	}
	c := EncoderCodec(f.Codec)
	for _, ok := range probeOrder {
		if ok == c {
			log.Printf("agent: using cached encoder %s (probed %.1f fps)", f.Label, f.ProbedFPS)
			return c, true
		}
	}
	return "", false
}

func saveCachedEncoderCodec(codec EncoderCodec, fps float64) {
	dir := filepath.Dir(encoderCachePath())
	_ = os.MkdirAll(dir, 0o755)
	f := encoderCacheFile{
		Version:   encoderCacheVersion,
		Codec:     string(codec),
		Label:     codec.label(),
		ProbedFPS: fps,
		ProbedAt:  time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(encoderCachePath(), b, 0o644); err != nil {
		log.Printf("agent: encoder cache write: %v", err)
		return
	}
	log.Printf("agent: encoder cached %s (%.1f fps) -> %s", f.Label, fps, encoderCachePath())
}
