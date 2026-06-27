#include "qsv_encode.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void qsv_set_error(QsvEncoder* enc, const char* msg) {
    if (!enc) return;
    snprintf(enc->last_error, sizeof(enc->last_error), "%s", msg ? msg : "");
}

static void qsv_set_errorf(QsvEncoder* enc, const char* fmt, int a, int b) {
    if (!enc) return;
    snprintf(enc->last_error, sizeof(enc->last_error), fmt, a, b);
}

static int try_load_library(QsvEncoder* enc, const char* path) {
    enc->lib = LoadLibraryA(path);
    return enc->lib != NULL;
}

static mfxStatus load_mfx(QsvEncoder* enc) {
    const char* names[] = {"libmfxhw64.dll", "libmfx64-gen.dll", "libmfx.dll"};

    for (int i = 0; i < 3; i++) {
        if (try_load_library(enc, names[i])) {
            goto loaded;
        }
    }

    char sysdir[MAX_PATH];
    UINT n = GetSystemDirectoryA(sysdir, MAX_PATH);
    if (n > 0) {
        for (int i = 0; i < 3; i++) {
            char full[MAX_PATH];
            snprintf(full, sizeof(full), "%s\\%s", sysdir, names[i]);
            if (try_load_library(enc, full)) {
                goto loaded;
            }
        }
    }

    qsv_set_error(enc, "Intel Media SDK DLL not found");
    return -1;

loaded:
#define LOAD(name) \
    enc->name = (name##_t)GetProcAddress(enc->lib, #name); \
    if (!enc->name) { \
        qsv_set_error(enc, "missing export " #name); \
        return -1; \
    }

    LOAD(MFXInit);
    LOAD(MFXClose);
    LOAD(MFXVideoCORE_SetHandle);
    LOAD(MFXVideoENCODE_Query);
    LOAD(MFXVideoENCODE_Init);
    LOAD(MFXVideoENCODE_Close);
    LOAD(MFXVideoENCODE_EncodeFrameAsync);
    LOAD(MFXVideoCORE_SyncOperation);
    enc->MFXVideoENCODE_Reset = (MFXVideoENCODE_Reset_t)GetProcAddress(enc->lib, "MFXVideoENCODE_Reset");
#undef LOAD
    return MFX_ERR_NONE;
}

static int qsv_open_session(QsvEncoder* enc) {
    CoInitializeEx(NULL, COINIT_MULTITHREADED);

    mfxVersion ver = {0, 1};
    mfxU32 impls[] = {
        MFX_IMPL_HARDWARE | MFX_IMPL_VIA_D3D11,
        MFX_IMPL_HARDWARE,
        MFX_IMPL_HARDWARE_ANY | MFX_IMPL_VIA_D3D11,
        MFX_IMPL_HARDWARE2 | MFX_IMPL_VIA_D3D11,
    };

    for (int i = 0; i < 4; i++) {
        enc->session = NULL;
        mfxStatus sts = enc->MFXInit((mfxIMPL)impls[i], &ver, &enc->session);
        if (sts >= MFX_ERR_NONE && enc->session) {
            return 0;
        }
        qsv_set_errorf(enc, "MFXInit failed sts=%d impl=0x%x", (int)sts, (int)impls[i]);
        if (enc->session) {
            enc->MFXClose(enc->session);
            enc->session = NULL;
        }
    }
    return -1;
}

static int is_annexb(const uint8_t* data, int size) {
    return size >= 4 && data[0] == 0 && data[1] == 0 && (data[2] == 1 || (data[2] == 0 && data[3] == 1));
}

static uint8_t* bitstream_to_annexb(const uint8_t* data, int size, int* out_size) {
    if (!data || size <= 0 || !out_size) return NULL;

    if (is_annexb(data, size)) {
        uint8_t* copy = (uint8_t*)malloc((size_t)size);
        if (!copy) return NULL;
        memcpy(copy, data, (size_t)size);
        *out_size = size;
        return copy;
    }

    int cap = size + 64;
    uint8_t* out = (uint8_t*)malloc((size_t)cap);
    if (!out) return NULL;

    int pos = 0;
    int off = 0;
    while (off + 4 <= size) {
        uint32_t nlen = ((uint32_t)data[off] << 24) | ((uint32_t)data[off + 1] << 16) |
                        ((uint32_t)data[off + 2] << 8) | (uint32_t)data[off + 3];
        off += 4;
        if (nlen == 0 || off + (int)nlen > size) break;
        if (pos + 4 + (int)nlen > cap) {
            cap = pos + 4 + (int)nlen + 256;
            uint8_t* grown = (uint8_t*)realloc(out, (size_t)cap);
            if (!grown) {
                free(out);
                return NULL;
            }
            out = grown;
        }
        out[pos++] = 0;
        out[pos++] = 0;
        out[pos++] = 0;
        out[pos++] = 1;
        memcpy(out + pos, data + off, nlen);
        pos += (int)nlen;
        off += (int)nlen;
    }

    if (pos == 0) {
        free(out);
        uint8_t* copy = (uint8_t*)malloc((size_t)size);
        if (!copy) return NULL;
        memcpy(copy, data, (size_t)size);
        *out_size = size;
        return copy;
    }
    *out_size = pos;
    return out;
}

static int qsv_try_init_encoder(QsvEncoder* enc);

int qsv_init(QsvEncoder* enc, ID3D11Device* d3d_device, int width, int height, int fps, int bitrate_kbps) {
    ZeroMemory(enc, sizeof(*enc));
    enc->width = width;
    enc->height = height;
    enc->fps = fps > 0 ? fps : 30;
    enc->bitrate_kbps = bitrate_kbps > 0 ? bitrate_kbps : 4000;
    enc->force_idr = 1;
    enc->priming = 1;
    snprintf(enc->name, sizeof(enc->name), "dxgi-qsv-h264");

    if (!d3d_device) {
        qsv_set_error(enc, "D3D11 device required");
        return -1;
    }
    if (load_mfx(enc) != MFX_ERR_NONE) {
        return -2;
    }

    if (qsv_open_session(enc) != 0) {
        return -3;
    }

    /* System-memory NV12 input does not require D3D11 SetHandle on many Intel drivers. */
    if (d3d_device) {
        mfxStatus sts = enc->MFXVideoCORE_SetHandle(enc->session, MFX_HANDLE_D3D11_DEVICE, (mfxMemId)d3d_device);
        if (sts < MFX_ERR_NONE) {
            fprintf(stderr, "connect: SetHandle skipped/failed sts=%d (continuing with system memory)\n", sts);
            fflush(stderr);
        }
    }

    mfxU16 aw = mfx_align16((mfxU16)width);
    mfxU16 ah = mfx_align16((mfxU16)height);

    ZeroMemory(&enc->param, sizeof(enc->param));
    enc->param.AsyncDepth = 1;
    enc->param.IOPattern = MFX_IOPATTERN_IN_SYSTEM_MEMORY;
    enc->param.ExtParam = NULL;
    enc->param.NumExtParam = 0;
    enc->param.mfx.FrameInfo.FourCC = MFX_FOURCC_NV12;
    enc->param.mfx.FrameInfo.BitDepthLuma = 8;
    enc->param.mfx.FrameInfo.BitDepthChroma = 8;
    enc->param.mfx.FrameInfo.Width = aw;
    enc->param.mfx.FrameInfo.Height = ah;
    enc->param.mfx.FrameInfo.CropW = (mfxU16)width;
    enc->param.mfx.FrameInfo.CropH = (mfxU16)height;
    enc->param.mfx.FrameInfo.ChromaFormat = MFX_CHROMAFORMAT_YUV420;
    enc->param.mfx.FrameInfo.PicStruct = MFX_PICSTRUCT_PROGRESSIVE;
    enc->param.mfx.FrameInfo.FrameRateExtN = (mfxU32)enc->fps;
    enc->param.mfx.FrameInfo.FrameRateExtD = 1;
    enc->param.mfx.LowPower = 1;
    enc->param.mfx.CodecId = MFX_CODEC_AVC;
    enc->param.mfx.CodecProfile = MFX_PROFILE_AVC_BASELINE;
    enc->param.mfx.CodecLevel = 0;
    enc->param.mfx.u.enc.TargetUsage = MFX_TARGETUSAGE_BEST_SPEED;
    enc->param.mfx.u.enc.RateControlMethod = MFX_RATECONTROL_CBR;
    enc->param.mfx.u.enc.TargetKbps = (mfxU16)enc->bitrate_kbps;
    enc->param.mfx.u.enc.MaxKbps = (mfxU16)enc->bitrate_kbps;
    enc->param.mfx.u.enc.BufferSizeInKB = (mfxU16)(enc->bitrate_kbps * 2 / 8 + 16);
    enc->param.mfx.u.enc.GopPicSize = (mfxU16)(enc->fps * 2);
    enc->param.mfx.u.enc.GopRefDist = 1;
    enc->param.mfx.u.enc.IdrInterval = 0;
    enc->param.mfx.u.enc.NumRefFrame = 1;
    enc->param.mfx.u.enc.NumSlice = 1;

    int rc = qsv_try_init_encoder(enc);
    if (rc != 0) {
        return rc;
    }

    enc->bitstream.MaxLength = (mfxU32)(enc->bitrate_kbps * 1024 / 8 + 65536);
    enc->bitstream.Data = (mfxU8*)malloc(enc->bitstream.MaxLength);
    if (!enc->bitstream.Data) {
        qsv_set_error(enc, "bitstream alloc failed");
        return -7;
    }
    qsv_set_error(enc, "");
    return 0;
}

static int qsv_try_init_encoder(QsvEncoder* enc) {
    struct {
        int low_power;
        mfxU16 level;
        int gop_div;
    } tries[] = {
        {0, MFX_LEVEL_AVC_40, 2},
        {0, 41, 2},
        {0, 0, 2},
        {1, MFX_LEVEL_AVC_40, 4},
        {0, MFX_LEVEL_AVC_40, 4},
    };

    for (int i = 0; i < 5; i++) {
        enc->param.mfx.LowPower = (mfxU16)tries[i].low_power;
        enc->param.mfx.CodecLevel = tries[i].level;
        enc->param.mfx.u.enc.GopPicSize = (mfxU16)(enc->fps / (tries[i].gop_div > 0 ? tries[i].gop_div : 2));
        enc->param.mfx.u.enc.GopRefDist = 1;
        enc->param.mfx.u.enc.IdrInterval = 1;
        enc->param.mfx.u.enc.BufferSizeInKB = (mfxU16)(enc->bitrate_kbps / 4 + 4);
        enc->param.mfx.u.enc.InitialDelayInKB = 0;
        enc->param.mfx.u.enc.NumRefFrame = 1;

        mfxVideoParam valid = enc->param;
        mfxStatus sts = enc->MFXVideoENCODE_Query(enc->session, &enc->param, &valid);
        if (sts < MFX_ERR_NONE) {
            qsv_set_errorf(enc, "ENCODE_Query failed sts=%d", (int)sts, 0);
            continue;
        }
        enc->param = valid;

        sts = enc->MFXVideoENCODE_Init(enc->session, &enc->param);
        if (sts >= MFX_ERR_NONE) {
            qsv_set_error(enc, "");
            return 0;
        }
        qsv_set_errorf(enc, "ENCODE_Init failed sts=%d", (int)sts, 0);
        if (enc->MFXVideoENCODE_Close) {
            enc->MFXVideoENCODE_Close(enc->session);
        }
    }
    return -6;
}

int qsv_encode(QsvEncoder* enc, const uint8_t* nv12, int nv12_size, int pitch, int force_idr, uint8_t** out_data, int* out_size, int* out_keyframe) {
    if (!enc || !enc->session || !nv12 || !out_data || !out_size || !out_keyframe) return -1;
    (void)nv12_size;

    mfxFrameSurface1 surface;
    ZeroMemory(&surface, sizeof(surface));
    surface.Info = enc->param.mfx.FrameInfo;
    surface.Data.Y = (mfxU8*)nv12;
    surface.Data.UV = (mfxU8*)(nv12 + (size_t)pitch * (size_t)enc->param.mfx.FrameInfo.Height);
    surface.Data.Pitch = (mfxU16)pitch;

    mfxBitstream bs;
    ZeroMemory(&bs, sizeof(bs));
    bs.MaxLength = enc->bitstream.MaxLength;
    bs.Data = enc->bitstream.Data;

    mfxEncodeCtrl ctrl;
    ZeroMemory(&ctrl, sizeof(ctrl));
    mfxEncodeCtrl* pctrl = NULL;
    if (force_idr || enc->force_idr) {
        ctrl.FrameType = MFX_FRAMETYPE_I | MFX_FRAMETYPE_IDR | MFX_FRAMETYPE_REF;
        pctrl = &ctrl;
        enc->force_idr = 0;
    }

    void* syncp = NULL;
    mfxStatus sts = MFX_ERR_NOT_ENOUGH_BUFFER;
    for (int tries = 0; tries < 32; tries++) {
        sts = enc->MFXVideoENCODE_EncodeFrameAsync(enc->session, pctrl, &surface, &bs, &syncp);
        if (sts != MFX_WRN_DEVICE_BUSY) break;
    }
    if (sts < MFX_ERR_NONE && sts != MFX_ERR_MORE_DATA) {
        qsv_set_error(enc, "EncodeFrameAsync failed");
        return -2;
    }
    if (syncp) {
        enc->MFXVideoCORE_SyncOperation(enc->session, syncp, 60000);
    }
    if (bs.DataLength == 0) {
        return 1;
    }

    enc->priming = 0;
    int raw_size = (int)bs.DataLength;
    uint8_t* annexb = bitstream_to_annexb(bs.Data + bs.DataOffset, raw_size, &raw_size);
    if (!annexb) return -3;

    *out_size = raw_size;
    *out_data = annexb;
    *out_keyframe = (bs.FrameType & (MFX_FRAMETYPE_IDR | MFX_FRAMETYPE_I)) ? 1 : 0;
    enc->frame_index++;
    return 0;
}

int qsv_set_bitrate(QsvEncoder* enc, int kbps) {
    if (!enc || !enc->session) return -1;
    enc->bitrate_kbps = kbps;
    enc->param.mfx.u.enc.TargetKbps = (mfxU16)kbps;
    enc->param.mfx.u.enc.MaxKbps = (mfxU16)kbps;
    enc->param.mfx.u.enc.BufferSizeInKB = (mfxU16)(kbps / 8 + 1);
    if (enc->MFXVideoENCODE_Reset) {
        mfxStatus sts = enc->MFXVideoENCODE_Reset(enc->session, &enc->param);
        if (sts < MFX_ERR_NONE) {
            return -2;
        }
    }
    enc->force_idr = 1;
    return 0;
}

const char* qsv_last_error(QsvEncoder* enc) {
    if (!enc || enc->last_error[0] == '\0') return "unknown qsv error";
    return enc->last_error;
}

void qsv_shutdown(QsvEncoder* enc) {
    if (!enc) return;
    if (enc->session && enc->MFXVideoENCODE_Close) {
        enc->MFXVideoENCODE_Close(enc->session);
    }
    if (enc->session && enc->MFXClose) {
        enc->MFXClose(enc->session);
    }
    if (enc->bitstream.Data) {
        free(enc->bitstream.Data);
        enc->bitstream.Data = NULL;
    }
    if (enc->lib) {
        FreeLibrary(enc->lib);
        enc->lib = NULL;
    }
    enc->session = NULL;
}
