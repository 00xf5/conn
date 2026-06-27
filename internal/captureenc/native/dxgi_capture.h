#ifndef CONNECT_DXGI_CAPTURE_H
#define CONNECT_DXGI_CAPTURE_H

#include "platform.h"

typedef struct {
    ID3D11Device* device;
    ID3D11DeviceContext* context;
    IDXGIOutputDuplication* duplication;
    ID3D11Texture2D* staging_bgra;
    ID3D11Texture2D* bgra_readback;
    ID3D11Texture2D* nv12;
    ID3D11VideoDevice* video_device;
    ID3D11VideoContext* video_context;
    ID3D11VideoProcessor* video_processor;
    ID3D11VideoProcessorEnumerator* video_enum;
    ID3D11VideoProcessorInputView* input_view;
    ID3D11VideoProcessorOutputView* output_view;
    int native_width;
    int native_height;
    int width;
    int height;
    int fps;
    ID3D11Texture2D* nv12_staging;
    uint8_t* nv12_cpu;
    size_t nv12_cpu_size;
    int nv12_pitch;
} DxgiCapture;

// Returns 0 on success.
int dxgi_capture_init(DxgiCapture* cap, int monitor_index, int want_width, int want_height, int fps);

// Returns 0 got frame, 1 timeout/no frame, <0 error (-2 access lost).
int dxgi_capture_acquire(DxgiCapture* cap, ID3D11Texture2D** out_bgra);

void dxgi_capture_release(DxgiCapture* cap);

// Converts latest acquired frame to NV12 texture (must call after acquire, before release).
int dxgi_capture_convert_nv12(DxgiCapture* cap, ID3D11Texture2D* bgra);

// Reads GPU NV12 into internal CPU buffer. Returns 0 on success.
int dxgi_capture_map_nv12(DxgiCapture* cap);

void dxgi_capture_unmap_nv12(DxgiCapture* cap);

const uint8_t* dxgi_capture_nv12_bytes(DxgiCapture* cap, int* pitch);

void dxgi_capture_shutdown(DxgiCapture* cap);

#endif
