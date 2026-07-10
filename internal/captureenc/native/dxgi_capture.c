#ifndef CONNECT_DXGI_CAPTURE_C
#define CONNECT_DXGI_CAPTURE_C

#include "dxgi_capture.h"

#include <stdlib.h>
#include <string.h>

static int pick_output(int monitor_index, IDXGIAdapter** out_adapter, IDXGIOutput** out_output, int* out_global_idx) {
    IDXGIFactory1* factory = NULL;
    if (FAILED(CreateDXGIFactory1(&IID_IDXGIFactory1, (void**)&factory))) {
        return -1;
    }

    int global = 0;
    int found = 0;
    UINT ai = 0;
    IDXGIAdapter* adapter = NULL;
    while (!found && factory->lpVtbl->EnumAdapters(factory, ai, &adapter) != DXGI_ERROR_NOT_FOUND) {
        UINT oi = 0;
        IDXGIOutput* output = NULL;
        while (!found && adapter->lpVtbl->EnumOutputs(adapter, oi, &output) != DXGI_ERROR_NOT_FOUND) {
            if (global == monitor_index) {
                *out_adapter = adapter;
                *out_output = output;
                *out_global_idx = global;
                found = 1;
            } else {
                output->lpVtbl->Release(output);
                global++;
            }
            oi++;
        }
        if (!found) {
            adapter->lpVtbl->Release(adapter);
            adapter = NULL;
        }
        ai++;
    }
    factory->lpVtbl->Release(factory);
    return found ? 0 : -2;
}

static inline uint8_t bgra_to_y(uint8_t b, uint8_t g, uint8_t r) {
    int y = (66 * (int)r + 129 * (int)g + 25 * (int)b + 128) >> 8;
    y += 16;
    if (y < 0) y = 0;
    if (y > 255) y = 255;
    return (uint8_t)y;
}

static inline void bgra_to_uv(uint8_t b, uint8_t g, uint8_t r, uint8_t* u, uint8_t* v) {
    int uu = (-38 * (int)r - 74 * (int)g + 112 * (int)b + 128) >> 8;
    int vv = (112 * (int)r - 94 * (int)g - 18 * (int)b + 128) >> 8;
    uu += 128;
    vv += 128;
    if (uu < 0) uu = 0;
    if (uu > 255) uu = 255;
    if (vv < 0) vv = 0;
    if (vv > 255) vv = 255;
    *u = (uint8_t)uu;
    *v = (uint8_t)vv;
}

static void convert_bgra_to_nv12_cpu(
    const uint8_t* bgra, int src_pitch, int src_w, int src_h,
    uint8_t* nv12, int dst_pitch, int dst_w, int dst_h) {
    for (int y = 0; y < dst_h; y++) {
        int sy = y * src_h / dst_h;
        const uint8_t* row = bgra + sy * src_pitch;
        uint8_t* yrow = nv12 + y * dst_pitch;
        for (int x = 0; x < dst_w; x++) {
            int sx = x * src_w / dst_w;
            const uint8_t* px = row + sx * 4;
            yrow[x] = bgra_to_y(px[0], px[1], px[2]);
        }
    }

    uint8_t* uv = nv12 + dst_pitch * dst_h;
    for (int y = 0; y < dst_h / 2; y++) {
        int sy = (y * 2) * src_h / dst_h;
        const uint8_t* row = bgra + sy * src_pitch;
        uint8_t* uvrow = uv + y * dst_pitch;
        for (int x = 0; x < dst_w / 2; x++) {
            int sx = (x * 2) * src_w / dst_w;
            const uint8_t* px = row + sx * 4;
            bgra_to_uv(px[0], px[1], px[2], &uvrow[x * 2], &uvrow[x * 2 + 1]);
        }
    }
}

static int create_video_processor(DxgiCapture* cap) {
    HRESULT hr;
    hr = cap->device->lpVtbl->QueryInterface(cap->device, &IID_ID3D11VideoDevice, (void**)&cap->video_device);
    if (FAILED(hr)) return -1;
    hr = cap->context->lpVtbl->QueryInterface(cap->context, &IID_ID3D11VideoContext, (void**)&cap->video_context);
    if (FAILED(hr)) return -2;

    D3D11_VIDEO_PROCESSOR_CONTENT_DESC desc;
    ZeroMemory(&desc, sizeof(desc));
    desc.InputFrameFormat = D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE;
    desc.InputWidth = cap->native_width;
    desc.InputHeight = cap->native_height;
    desc.OutputWidth = cap->width;
    desc.OutputHeight = cap->height;
    desc.Usage = D3D11_VIDEO_USAGE_PLAYBACK_NORMAL;

    hr = cap->video_device->lpVtbl->CreateVideoProcessorEnumerator(cap->video_device, &desc, &cap->video_enum);
    if (FAILED(hr)) return -3;
    hr = cap->video_device->lpVtbl->CreateVideoProcessor(cap->video_device, cap->video_enum, 0, &cap->video_processor);
    if (FAILED(hr)) return -4;

    D3D11_TEXTURE2D_DESC nv12_desc;
    ZeroMemory(&nv12_desc, sizeof(nv12_desc));
    nv12_desc.Width = cap->width;
    nv12_desc.Height = cap->height;
    nv12_desc.MipLevels = 1;
    nv12_desc.ArraySize = 1;
    nv12_desc.Format = DXGI_FORMAT_NV12;
    nv12_desc.SampleDesc.Count = 1;
    nv12_desc.Usage = D3D11_USAGE_DEFAULT;
    nv12_desc.BindFlags = D3D11_BIND_RENDER_TARGET;
    hr = cap->device->lpVtbl->CreateTexture2D(cap->device, &nv12_desc, NULL, &cap->nv12);
    if (FAILED(hr)) return -5;

    D3D11_VIDEO_PROCESSOR_OUTPUT_VIEW_DESC out_view_desc;
    ZeroMemory(&out_view_desc, sizeof(out_view_desc));
    out_view_desc.ViewDimension = D3D11_VPOV_DIMENSION_TEXTURE2D;
    out_view_desc.Texture2D.MipSlice = 0;
    hr = cap->video_device->lpVtbl->CreateVideoProcessorOutputView(
        cap->video_device, (ID3D11Resource*)cap->nv12, cap->video_enum, &out_view_desc, &cap->output_view);
    if (FAILED(hr)) return -6;

    return 0;
}

static int create_capture_buffers(DxgiCapture* cap) {
    HRESULT hr;
    D3D11_TEXTURE2D_DESC bgra_desc;
    ZeroMemory(&bgra_desc, sizeof(bgra_desc));
    bgra_desc.Width = cap->native_width;
    bgra_desc.Height = cap->native_height;
    bgra_desc.MipLevels = 1;
    bgra_desc.ArraySize = 1;
    bgra_desc.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
    bgra_desc.SampleDesc.Count = 1;
    bgra_desc.Usage = D3D11_USAGE_DEFAULT;
    bgra_desc.BindFlags = D3D11_BIND_SHADER_RESOURCE;
    hr = cap->device->lpVtbl->CreateTexture2D(cap->device, &bgra_desc, NULL, &cap->staging_bgra);
    if (FAILED(hr)) return -1;

    bgra_desc.Usage = D3D11_USAGE_STAGING;
    bgra_desc.BindFlags = 0;
    bgra_desc.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
    hr = cap->device->lpVtbl->CreateTexture2D(cap->device, &bgra_desc, NULL, &cap->bgra_readback);
    if (FAILED(hr)) return -2;

    cap->nv12_pitch = (cap->width + 1) & ~1;
    cap->nv12_cpu_size = (size_t)cap->nv12_pitch * (size_t)cap->height * 3 / 2;
    cap->nv12_cpu = (uint8_t*)malloc(cap->nv12_cpu_size);
    if (!cap->nv12_cpu) return -3;

    return 0;
}

int dxgi_capture_init(DxgiCapture* cap, int monitor_index, int want_width, int want_height, int fps) {
    ZeroMemory(cap, sizeof(*cap));
    cap->fps = fps > 0 ? fps : 30;

    IDXGIAdapter* adapter = NULL;
    IDXGIOutput* output = NULL;
    int global_idx = 0;
    if (pick_output(monitor_index, &adapter, &output, &global_idx) != 0) {
        return -10;
    }

    D3D_FEATURE_LEVEL levels[] = {D3D_FEATURE_LEVEL_11_1, D3D_FEATURE_LEVEL_11_0};
    D3D_FEATURE_LEVEL chosen;
    HRESULT hr = D3D11CreateDevice(
        adapter,
        D3D_DRIVER_TYPE_UNKNOWN,
        NULL,
        D3D11_CREATE_DEVICE_VIDEO_SUPPORT,
        levels,
        2,
        D3D11_SDK_VERSION,
        &cap->device,
        &chosen,
        &cap->context);
    if (FAILED(hr)) {
        output->lpVtbl->Release(output);
        adapter->lpVtbl->Release(adapter);
        return -11;
    }

    DXGI_OUTPUT_DESC out_desc;
    output->lpVtbl->GetDesc(output, &out_desc);
    int native_w = out_desc.DesktopCoordinates.right - out_desc.DesktopCoordinates.left;
    int native_h = out_desc.DesktopCoordinates.bottom - out_desc.DesktopCoordinates.top;
    cap->width = native_w;
    cap->height = native_h;
    cap->native_width = native_w;
    cap->native_height = native_h;
    if (want_width > 0 && want_height > 0) {
        cap->width = want_width;
        cap->height = want_height;
    }

    IDXGIOutput1* output1 = NULL;
    hr = output->lpVtbl->QueryInterface(output, &IID_IDXGIOutput1, (void**)&output1);
    output->lpVtbl->Release(output);
    if (FAILED(hr)) {
        adapter->lpVtbl->Release(adapter);
        return -12;
    }

    hr = output1->lpVtbl->DuplicateOutput(output1, (IUnknown*)cap->device, &cap->duplication);
    output1->lpVtbl->Release(output1);
    adapter->lpVtbl->Release(adapter);
    if (FAILED(hr)) {
        return -13;
    }

    if (create_capture_buffers(cap) != 0) {
        dxgi_capture_shutdown(cap);
        return -14;
    }

    return 0;
}

static int ensure_staging_textures(DxgiCapture* cap, ID3D11Texture2D* frame) {
    D3D11_TEXTURE2D_DESC fd;
    frame->lpVtbl->GetDesc(frame, &fd);

    D3D11_TEXTURE2D_DESC sd;
    ZeroMemory(&sd, sizeof(sd));
    if (cap->staging_bgra) {
        cap->staging_bgra->lpVtbl->GetDesc(cap->staging_bgra, &sd);
    }

    if (cap->staging_bgra && sd.Width == fd.Width && sd.Height == fd.Height) {
        return 0;
    }

    if (cap->bgra_readback) {
        cap->bgra_readback->lpVtbl->Release(cap->bgra_readback);
        cap->bgra_readback = NULL;
    }
    if (cap->staging_bgra) {
        cap->staging_bgra->lpVtbl->Release(cap->staging_bgra);
        cap->staging_bgra = NULL;
    }

    cap->native_width = (int)fd.Width;
    cap->native_height = (int)fd.Height;
    if (cap->width <= 0 || cap->height <= 0) {
        cap->width = cap->native_width;
        cap->height = cap->native_height;
    }

    HRESULT hr;
    D3D11_TEXTURE2D_DESC bgra_desc;
    ZeroMemory(&bgra_desc, sizeof(bgra_desc));
    bgra_desc.Width = fd.Width;
    bgra_desc.Height = fd.Height;
    bgra_desc.MipLevels = 1;
    bgra_desc.ArraySize = 1;
    bgra_desc.Format = fd.Format;
    bgra_desc.SampleDesc.Count = 1;
    bgra_desc.Usage = D3D11_USAGE_DEFAULT;
    bgra_desc.BindFlags = D3D11_BIND_SHADER_RESOURCE;
    hr = cap->device->lpVtbl->CreateTexture2D(cap->device, &bgra_desc, NULL, &cap->staging_bgra);
    if (FAILED(hr)) return -1;

    bgra_desc.Usage = D3D11_USAGE_STAGING;
    bgra_desc.BindFlags = 0;
    bgra_desc.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
    hr = cap->device->lpVtbl->CreateTexture2D(cap->device, &bgra_desc, NULL, &cap->bgra_readback);
    if (FAILED(hr)) return -2;

    cap->nv12_pitch = (cap->width + 1) & ~1;
    size_t need = (size_t)cap->nv12_pitch * (size_t)cap->height * 3 / 2;
    if (!cap->nv12_cpu || cap->nv12_cpu_size < need) {
        free(cap->nv12_cpu);
        cap->nv12_cpu = (uint8_t*)malloc(need);
        if (!cap->nv12_cpu) return -3;
        cap->nv12_cpu_size = need;
    }
    return 0;
}

int dxgi_capture_acquire(DxgiCapture* cap, ID3D11Texture2D** out_bgra) {
    if (!cap || !cap->duplication || !out_bgra) return -1;

    IDXGIResource* resource = NULL;
    DXGI_OUTDUPL_FRAME_INFO info;
    ZeroMemory(&info, sizeof(info));

    HRESULT hr = cap->duplication->lpVtbl->AcquireNextFrame(cap->duplication, 250, &info, &resource);
    if (hr == DXGI_ERROR_WAIT_TIMEOUT) {
        return 1;
    }
    if (hr == DXGI_ERROR_ACCESS_LOST) {
        return -2;
    }
    if (FAILED(hr)) {
        return -3;
    }

    ID3D11Texture2D* frame = NULL;
    hr = resource->lpVtbl->QueryInterface(resource, &IID_ID3D11Texture2D, (void**)&frame);
    resource->lpVtbl->Release(resource);
    if (FAILED(hr)) {
        cap->duplication->lpVtbl->ReleaseFrame(cap->duplication);
        return -4;
    }

    if (ensure_staging_textures(cap, frame) != 0) {
        frame->lpVtbl->Release(frame);
        cap->duplication->lpVtbl->ReleaseFrame(cap->duplication);
        return -5;
    }

    cap->context->lpVtbl->CopyResource(cap->context, (ID3D11Resource*)cap->staging_bgra, (ID3D11Resource*)frame);
    frame->lpVtbl->Release(frame);
    cap->duplication->lpVtbl->ReleaseFrame(cap->duplication);

    *out_bgra = cap->staging_bgra;
    return 0;
}

int dxgi_capture_convert_nv12(DxgiCapture* cap, ID3D11Texture2D* bgra) {
    if (!cap || !cap->staging_bgra || !cap->bgra_readback || !cap->nv12_cpu) return -1;
    if (bgra && bgra != cap->staging_bgra) return -1;

    cap->context->lpVtbl->CopyResource(cap->context, (ID3D11Resource*)cap->bgra_readback, (ID3D11Resource*)cap->staging_bgra);

    D3D11_MAPPED_SUBRESOURCE mapped;
    HRESULT hr = cap->context->lpVtbl->Map(cap->context, (ID3D11Resource*)cap->bgra_readback, 0, D3D11_MAP_READ, 0, &mapped);
    if (FAILED(hr)) return -2;

    convert_bgra_to_nv12_cpu(
        (const uint8_t*)mapped.pData, (int)mapped.RowPitch, cap->native_width, cap->native_height,
        cap->nv12_cpu, cap->nv12_pitch, cap->width, cap->height);
    cap->context->lpVtbl->Unmap(cap->context, (ID3D11Resource*)cap->bgra_readback, 0);
    return 0;
}

void dxgi_capture_release(DxgiCapture* cap) {
    if (!cap) return;
    dxgi_capture_unmap_nv12(cap);
}

int dxgi_capture_map_nv12(DxgiCapture* cap) {
    if (!cap || !cap->nv12_cpu) return -1;
    return 0;
}

void dxgi_capture_unmap_nv12(DxgiCapture* cap) {
    (void)cap;
}

const uint8_t* dxgi_capture_nv12_bytes(DxgiCapture* cap, int* pitch) {
    if (!cap || !cap->nv12_cpu) return NULL;
    if (pitch) *pitch = cap->nv12_pitch;
    return cap->nv12_cpu;
}

void dxgi_capture_shutdown(DxgiCapture* cap) {
    if (!cap) return;
    if (cap->input_view) cap->input_view->lpVtbl->Release(cap->input_view);
    if (cap->output_view) cap->output_view->lpVtbl->Release(cap->output_view);
    if (cap->video_processor) cap->video_processor->lpVtbl->Release(cap->video_processor);
    if (cap->video_enum) cap->video_enum->lpVtbl->Release(cap->video_enum);
    if (cap->video_context) cap->video_context->lpVtbl->Release(cap->video_context);
    if (cap->video_device) cap->video_device->lpVtbl->Release(cap->video_device);
    if (cap->nv12) cap->nv12->lpVtbl->Release(cap->nv12);
    free(cap->nv12_cpu);
    if (cap->bgra_readback) cap->bgra_readback->lpVtbl->Release(cap->bgra_readback);
    if (cap->staging_bgra) cap->staging_bgra->lpVtbl->Release(cap->staging_bgra);
    if (cap->duplication) cap->duplication->lpVtbl->Release(cap->duplication);
    if (cap->context) cap->context->lpVtbl->Release(cap->context);
    if (cap->device) cap->device->lpVtbl->Release(cap->device);
    ZeroMemory(cap, sizeof(*cap));
}

#endif /* CONNECT_DXGI_CAPTURE_C */
