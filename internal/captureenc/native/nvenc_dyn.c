#include "nvenc_dyn.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static NVENCSTATUS nv_call(NvencEncoder* enc, NVENCSTATUS st) {
    (void)enc;
    return st;
}

int nvenc_init(NvencEncoder* enc, ID3D11Device* device, int width, int height, int fps, int bitrate_kbps) {
    ZeroMemory(enc, sizeof(*enc));
    enc->width = width;
    enc->height = height;
    enc->fps = fps > 0 ? fps : 30;
    enc->bitrate_kbps = bitrate_kbps > 0 ? bitrate_kbps : 4000;
    enc->force_idr = 1;
    snprintf(enc->name, sizeof(enc->name), "nvenc-h264");

    enc->lib = LoadLibraryA("nvEncodeAPI64.dll");
    if (!enc->lib) {
        return -1;
    }

    NvEncodeAPICreateInstance_t create_fn =
        (NvEncodeAPICreateInstance_t)GetProcAddress(enc->lib, "NvEncodeAPICreateInstance");
    if (!create_fn) {
        return -2;
    }

    ZeroMemory(&enc->api, sizeof(enc->api));
    enc->api.version = NV_ENCODE_API_FUNCTION_LIST_VER;
    if (create_fn(&enc->api) != NV_ENC_SUCCESS) {
        return -3;
    }

    NV_ENC_OPEN_ENCODE_SESSIONEX_PARAMS ses;
    ZeroMemory(&ses, sizeof(ses));
    ses.version = NV_ENC_OPEN_ENCODE_SESSION_EX_PARAMS_VER;
    ses.deviceType = NV_ENC_DEVICE_TYPE_DIRECTX;
    ses.device = device;
    ses.apiVersion = NVENCAPI_VERSION;
    if (nv_call(enc, enc->api.nvEncOpenEncodeSessionEx(&ses, &enc->encoder)) != NV_ENC_SUCCESS) {
        return -4;
    }

    NV_ENC_CONFIG cfg;
    ZeroMemory(&cfg, sizeof(cfg));
    cfg.version = NV_ENC_CONFIG_VER;
    cfg.gopLength = (uint32_t)(enc->fps * 2);
    cfg.frameIntervalP = 1;
    cfg.rcParams.version = NV_ENC_RC_PARAMS_VER;
    cfg.rcParams.rateControlMode = NV_ENC_PARAMS_RC_CBR;
    cfg.rcParams.averageBitRate = (uint32_t)(enc->bitrate_kbps * 1000);
    cfg.rcParams.maxBitRate = (uint32_t)(enc->bitrate_kbps * 1000);
    cfg.rcParams.zeroReorderDelay = 1;
    cfg.rcParams.enableLookahead = 0;
    cfg.encodeCodecConfig.version = NV_ENC_CODEC_CONFIG_VER;
    cfg.encodeCodecConfig.h264Config.version = NV_ENC_CONFIG_H264_VER;
    cfg.encodeCodecConfig.h264Config.repeatSPSPPS = 1;
    cfg.encodeCodecConfig.h264Config.outputAUD = 1;
    cfg.encodeCodecConfig.h264Config.idrPeriod = cfg.gopLength;
    cfg.encodeCodecConfig.h264Config.maxNumRefFrames = 1;

    NV_ENC_INITIALIZE_PARAMS init;
    ZeroMemory(&init, sizeof(init));
    init.version = NV_ENC_INITIALIZE_PARAMS_VER;
    init.encodeGUID = NV_ENC_CODEC_H264_GUID;
    init.presetGUID = NV_ENC_PRESET_P4_GUID;
    init.encodeWidth = (uint32_t)width;
    init.encodeHeight = (uint32_t)height;
    init.darWidth = (uint32_t)width;
    init.darHeight = (uint32_t)height;
    init.frameRateNum = (uint32_t)enc->fps;
    init.frameRateDen = 1;
    init.enablePTD = 1;
    init.encodeConfig = &cfg;
    init.maxEncodeWidth = (uint32_t)width;
    init.maxEncodeHeight = (uint32_t)height;

    if (nv_call(enc, enc->api.nvEncInitializeEncoder(enc->encoder, &init)) != NV_ENC_SUCCESS) {
        return -5;
    }

    NV_ENC_CREATE_BITSTREAM_BUFFER bs;
    ZeroMemory(&bs, sizeof(bs));
    bs.version = NV_ENC_CREATE_BITSTREAM_BUFFER_VER;
    if (nv_call(enc, enc->api.nvEncCreateBitstreamBuffer(enc->encoder, &bs)) != NV_ENC_SUCCESS) {
        return -6;
    }
    enc->bitstream = bs.bitstreamBuffer;

    return 0;
}

int nvenc_register_texture(NvencEncoder* enc, ID3D11Texture2D* nv12) {
    if (!enc || !enc->encoder || !nv12) return -1;

    if (enc->registered) {
        if (enc->mapped) {
            enc->api.nvEncUnmapInputResource(enc->encoder, enc->mapped);
            enc->mapped = NULL;
        }
        enc->api.nvEncUnregisterResource(enc->encoder, enc->registered);
        enc->registered = NULL;
    }

    NV_ENC_REGISTER_RESOURCE reg;
    ZeroMemory(&reg, sizeof(reg));
    reg.version = NV_ENC_REGISTER_RESOURCE_VER;
    reg.resourceType = NV_ENC_INPUT_RESOURCE_TYPE_DIRECTX;
    reg.resourceToRegister = (NV_ENC_INPUT_PTR)nv12;
    reg.width = (uint32_t)enc->width;
    reg.height = (uint32_t)enc->height;
    reg.pitch = (uint32_t)enc->width;
    reg.bufferFormat = NV_ENC_BUFFER_FORMAT_NV12;
    reg.bufferUsage = 0;
    if (nv_call(enc, enc->api.nvEncRegisterResource(enc->encoder, &reg)) != NV_ENC_SUCCESS) {
        return -2;
    }
    enc->registered = reg.registeredResource;
    return 0;
}

int nvenc_encode(NvencEncoder* enc, int force_idr, uint8_t** out_data, int* out_size, int* out_keyframe) {
    if (!enc || !enc->encoder || !enc->registered) return -1;

    NV_ENC_MAP_INPUT_RESOURCE map;
    ZeroMemory(&map, sizeof(map));
    map.version = NV_ENC_MAP_INPUT_RESOURCE_VER;
    map.registeredResource = enc->registered;
    if (nv_call(enc, enc->api.nvEncMapInputResource(enc->encoder, &map)) != NV_ENC_SUCCESS) {
        return -2;
    }
    enc->mapped = map.mappedResource;

    NV_ENC_PIC_PARAMS pic;
    ZeroMemory(&pic, sizeof(pic));
    pic.version = NV_ENC_PIC_PARAMS_VER;
    pic.inputWidth = (uint32_t)enc->width;
    pic.inputHeight = (uint32_t)enc->height;
    pic.inputPitch = (uint32_t)enc->width;
    pic.bufferFmt = NV_ENC_BUFFER_FORMAT_NV12;
    pic.inputBuffer = enc->mapped;
    pic.outputBitstream = enc->bitstream;
    pic.inputTimeStamp = (uint64_t)enc->frame_index;
    pic.frameIdx = (uint32_t)enc->frame_index;
    if (force_idr || enc->force_idr) {
        pic.encodePicFlags = NV_ENC_PIC_FLAG_FORCEIDR;
        enc->force_idr = 0;
    }

    if (nv_call(enc, enc->api.nvEncEncodePicture(enc->encoder, &pic)) != NV_ENC_SUCCESS) {
        enc->api.nvEncUnmapInputResource(enc->encoder, enc->mapped);
        enc->mapped = NULL;
        return -3;
    }

    NV_ENC_LOCK_BITSTREAM lock;
    ZeroMemory(&lock, sizeof(lock));
    lock.version = NV_ENC_LOCK_BITSTREAM_VER;
    lock.outputBitstream = enc->bitstream;
    lock.doNotWait = 0;
    if (nv_call(enc, enc->api.nvEncLockBitstream(enc->encoder, &lock)) != NV_ENC_SUCCESS) {
        enc->api.nvEncUnmapInputResource(enc->encoder, enc->mapped);
        enc->mapped = NULL;
        return -4;
    }

    *out_size = (int)lock.bitstreamSizeInBytes;
    *out_data = (uint8_t*)malloc(*out_size);
    if (!*out_data) {
        enc->api.nvEncUnlockBitstream(enc->encoder, enc->bitstream);
        enc->api.nvEncUnmapInputResource(enc->encoder, enc->mapped);
        enc->mapped = NULL;
        return -5;
    }
    memcpy(*out_data, lock.bitstreamBufferPtr, *out_size);
    *out_keyframe = (lock.pictureType == NV_ENC_PIC_TYPE_IDR) ? 1 : 0;

    enc->api.nvEncUnlockBitstream(enc->encoder, enc->bitstream);
    enc->api.nvEncUnmapInputResource(enc->encoder, enc->mapped);
    enc->mapped = NULL;
    enc->frame_index++;
    return 0;
}

int nvenc_set_bitrate(NvencEncoder* enc, int kbps) {
    if (!enc) return -1;
    enc->bitrate_kbps = kbps;
    enc->force_idr = 1;
    /* Full reconfigure omitted for v1; next IDR uses updated internal target via future nvEncReconfigureEncoder. */
    return 0;
}

void nvenc_shutdown(NvencEncoder* enc) {
    if (!enc) return;
    if (enc->encoder) {
        if (enc->mapped) {
            enc->api.nvEncUnmapInputResource(enc->encoder, enc->mapped);
            enc->mapped = NULL;
        }
        if (enc->registered) {
            enc->api.nvEncUnregisterResource(enc->encoder, enc->registered);
            enc->registered = NULL;
        }
        if (enc->bitstream) {
            enc->api.nvEncDestroyBitstreamBuffer(enc->encoder, enc->bitstream);
            enc->bitstream = NULL;
        }
        enc->api.nvEncDestroyEncoder(enc->encoder);
        enc->encoder = NULL;
    }
    if (enc->lib) {
        FreeLibrary(enc->lib);
        enc->lib = NULL;
    }
    ZeroMemory(enc, sizeof(*enc));
}
