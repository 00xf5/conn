#include "capture_only.h"

#include "dxgi_capture.h"

#include <stdlib.h>
#include <string.h>

typedef struct {
    CaptureOnlyConfig cfg;
    DxgiCapture dxgi;
    int need_reinit;
} CaptureOnlyState;

int capture_only_init(const CaptureOnlyConfig* cfg, CaptureOnlyHandle* out) {
    if (!cfg || !out) return -1;
    CaptureOnlyState* st = (CaptureOnlyState*)calloc(1, sizeof(CaptureOnlyState));
    if (!st) return -2;
    st->cfg = *cfg;
    if (dxgi_capture_init(&st->dxgi, cfg->monitor_index, cfg->width, cfg->height, cfg->fps) != 0) {
        free(st);
        return -10;
    }
    *out = (CaptureOnlyHandle)st;
    return 0;
}

int capture_only_read(CaptureOnlyHandle handle, CaptureOnlyFrame* out) {
    if (!handle || !out) return -1;
    CaptureOnlyState* st = (CaptureOnlyState*)handle;
    ZeroMemory(out, sizeof(*out));

    if (st->need_reinit || !st->dxgi.duplication) {
        dxgi_capture_shutdown(&st->dxgi);
        if (dxgi_capture_init(&st->dxgi, st->cfg.monitor_index, st->cfg.width, st->cfg.height, st->cfg.fps) != 0) {
            st->need_reinit = 1;
            Sleep(250); /* avoid busy-spin while locked */
            return 1; /* lock screen / secure desktop — soft pause */
        }
        st->need_reinit = 0;
    }

    ID3D11Texture2D* bgra = NULL;
    int acq = dxgi_capture_acquire(&st->dxgi, &bgra);
    if (acq == 1) return 1;
    if (acq == -2) {
        st->need_reinit = 1;
        return 1;
    }
    if (acq != 0) {
        st->need_reinit = 1;
        return 1;
    }

    if (dxgi_capture_convert_nv12(&st->dxgi, bgra) != 0) {
        dxgi_capture_release(&st->dxgi);
        return 1;
    }
    dxgi_capture_release(&st->dxgi);

    if (dxgi_capture_map_nv12(&st->dxgi) != 0) return 1;

    int pitch = 0;
    const uint8_t* nv12 = dxgi_capture_nv12_bytes(&st->dxgi, &pitch);
    if (!nv12) return 1;

    int h = st->dxgi.height;
    size_t size = (size_t)pitch * (size_t)h * 3 / 2;
    out->data = (uint8_t*)malloc(size);
    if (!out->data) return -6;
    memcpy(out->data, nv12, size);
    out->size = (int)size;
    out->pitch = pitch;
    out->width = st->dxgi.width;
    out->height = st->dxgi.height;
    return 0;
}

void capture_only_release(CaptureOnlyHandle handle, CaptureOnlyFrame* frame) {
    (void)handle;
    if (frame && frame->data) {
        free(frame->data);
        frame->data = NULL;
        frame->size = 0;
    }
}

void capture_only_shutdown(CaptureOnlyHandle handle) {
    if (!handle) return;
    CaptureOnlyState* st = (CaptureOnlyState*)handle;
    dxgi_capture_shutdown(&st->dxgi);
    free(st);
}
