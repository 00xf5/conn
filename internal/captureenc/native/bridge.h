#ifndef CONNECT_CAPTUREENC_BRIDGE_H
#define CONNECT_CAPTUREENC_BRIDGE_H

#include <stdint.h>

typedef void* CaptureEncHandle;

typedef struct {
    int monitor_index;
    int width;
    int height;
    int fps;
    int bitrate_kbps;
} CaptureEncConfig;

typedef struct {
    uint8_t* data;
    int size;
    int keyframe;
    uint64_t timestamp_us;
} CaptureEncFrame;

// Returns 0 on success.
int captureenc_init(const CaptureEncConfig* cfg, CaptureEncHandle* out);

// Returns 0 on frame, 1 if no frame ready (poll again), <0 on error.
int captureenc_read_frame(CaptureEncHandle handle, CaptureEncFrame* out);

void captureenc_release_frame(CaptureEncHandle handle, CaptureEncFrame* frame);

int captureenc_set_bitrate(CaptureEncHandle handle, int kbps);

int captureenc_request_keyframe(CaptureEncHandle handle);

const char* captureenc_encoder_name(CaptureEncHandle handle);

void captureenc_capture_size(CaptureEncHandle handle, int* width, int* height);

void captureenc_shutdown(CaptureEncHandle handle);

// Force DXGI/encoder reinit (e.g. after prolonged stall).
int captureenc_recover(CaptureEncHandle handle);

#endif
