#ifndef CONNECT_CAPTURE_ONLY_H
#define CONNECT_CAPTURE_ONLY_H

#include <stdint.h>

typedef void* CaptureOnlyHandle;

typedef struct {
    int monitor_index;
    int width;
    int height;
    int fps;
} CaptureOnlyConfig;

typedef struct {
    uint8_t* data;
    int size;
    int pitch;
    int width;
    int height;
} CaptureOnlyFrame;

int capture_only_init(const CaptureOnlyConfig* cfg, CaptureOnlyHandle* out);
int capture_only_read(CaptureOnlyHandle handle, CaptureOnlyFrame* out);
void capture_only_release(CaptureOnlyHandle handle, CaptureOnlyFrame* frame);
void capture_only_shutdown(CaptureOnlyHandle handle);

#endif
