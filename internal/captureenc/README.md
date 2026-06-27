# Capture (stable vs experimental)

## Stable (used by connect-agent when `CONNECT_ENCODER_DXGI=1`)

| File | Role |
|------|------|
| `capture_only.go` | Go wrapper |
| `native/capture_only.c` | DXGI → NV12 only |
| `native/dxgi_capture.c` | Desktop duplication |

Agent encodes via **ffmpeg pipe** with live-probed codec (`internal/agent/encoder_dxgi_ffmpeg_windows.go`): `h264_nvenc`, `h264_amf`, `h264_qsv`, or `libx264`.

**Default agent path does not use this package** — default is **gdigrab** in `internal/agent/encoder_ffmpeg_windows.go`.

## Experimental (not linked in default agent build)

Requires `-tags experimental`:

| File | Role |
|------|------|
| `encoder.go` | In-process H.264 encoder API |
| `native/bridge.c` | NVENC → QSV pipeline |
| `native/qsv_encode.c` | Intel QSV in-process |
| `native/nvenc_dyn.c` | NVIDIA NVENC |

Blocked on Intel UHD 605 (QSV init -15, driver crashes). Keep for future hardware testing only.

```powershell
go build -tags experimental -o captureenc-test.exe ./experimental/captureenc-test
```
