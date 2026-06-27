//go:build windows && cgo && experimental

package captureenc

/*
#cgo CFLAGS: -I${SRCDIR}/native
#cgo LDFLAGS: -ld3d11 -ldxgi -luuid -lole32

#include "bridge.h"
#include "bridge.c"
#include "dxgi_capture.c"
#include "nvenc_dyn.c"
#include "qsv_encode.c"
#include "capture_only.c"
*/
import "C"
