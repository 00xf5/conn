#ifndef CONNECT_NVENC_DYN_H
#define CONNECT_NVENC_DYN_H

#include "nvenc_minimal.h"
#include "platform.h"

typedef struct {
    HMODULE lib;
    NV_ENCODE_API_FUNCTION_LIST api;
    void* encoder;
    NV_ENC_OUTPUT_PTR bitstream;
    NV_ENC_INPUT_PTR registered;
    NV_ENC_INPUT_PTR mapped;
    int width;
    int height;
    int fps;
    int bitrate_kbps;
    int frame_index;
    int force_idr;
    char name[64];
} NvencEncoder;

int nvenc_init(NvencEncoder* enc, ID3D11Device* device, int width, int height, int fps, int bitrate_kbps);
int nvenc_register_texture(NvencEncoder* enc, ID3D11Texture2D* nv12);
int nvenc_encode(NvencEncoder* enc, int force_idr, uint8_t** out_data, int* out_size, int* out_keyframe);
int nvenc_set_bitrate(NvencEncoder* enc, int kbps);
void nvenc_shutdown(NvencEncoder* enc);

#endif
