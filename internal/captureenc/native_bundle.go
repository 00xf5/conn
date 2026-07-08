//go:build windows && cgo

package captureenc

/*
#cgo CFLAGS: -I${SRCDIR}/native
#cgo LDFLAGS: -ld3d11 -ldxgi -luuid -lole32 -lmfplat -lmfuuid -loleaut32

#include "bridge.c"
#include "dxgi_capture.c"
#include "nvenc_dyn.c"
#include "qsv_encode.c"
#include "capture_only.c"
#include "mf_h264_encode.c"
*/
import "C"
