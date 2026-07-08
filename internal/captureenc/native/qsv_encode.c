#include "qsv_encode.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef _WIN32
#include <windows.h>
#endif

static void qsv_set_error(QsvEncoder* enc, const char* msg) {
    if (!enc) return;
    snprintf(enc->last_error, sizeof(enc->last_error), "%s", msg ? msg : "");
}

static void qsv_set_errorf1(QsvEncoder* enc, const char* fmt, int a) {
    if (!enc) return;
    snprintf(enc->last_error, sizeof(enc->last_error), fmt, a);
}

static mfxU16 qsv_y_plane_height(const QsvEncoder* enc) {
    if (enc->param.mfx.FrameInfo.CropH > 0) {
        return enc->param.mfx.FrameInfo.CropH;
    }
    return (mfxU16)enc->height;
}

static int qsv_grow_bitstream(QsvEncoder* enc, mfxBitstream* bs) {
    mfxU32 new_max = enc->bitstream.MaxLength * 2;
    if (new_max < 262144) {
        new_max = 262144;
    }
    mfxU8* grown = (mfxU8*)realloc(enc->bitstream.Data, (size_t)new_max);
    if (!grown) {
        return -1;
    }
    enc->bitstream.Data = grown;
    enc->bitstream.MaxLength = new_max;
    bs->Data = enc->bitstream.Data;
    bs->MaxLength = new_max;
    return 0;
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
        snprintf(enc->last_error, sizeof(enc->last_error), "MFXInit failed sts=%d impl=0x%x", (int)sts, (int)impls[i]);
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

static mfxU32 qsv_avg_frame_bytes(const QsvEncoder* enc) {
    int fps = enc->fps > 0 ? enc->fps : 20;
    mfxU32 avg = (mfxU32)enc->bitrate_kbps * 1000U / (mfxU32)fps / 8U;
    return avg < 4096U ? 4096U : avg;
}

static uint8_t* bitstream_to_annexb(const uint8_t* data, int size, int* out_size, mfxU32 max_nal) {
    if (!data || size <= 0 || !out_size) return NULL;

    if (is_annexb(data, size)) {
        int end = size;
        while (end > 4 && data[end - 1] == 0) {
            end--;
        }
        if (end > (int)max_nal && max_nal > 0) {
            /* Keep full AU for keyframes — cap only obvious buffer padding. */
            end = size;
        }
        uint8_t* copy = (uint8_t*)malloc((size_t)end);
        if (!copy) return NULL;
        memcpy(copy, data, (size_t)end);
        *out_size = end;
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
        if (nlen == 0 || nlen > 256U * 1024U || off + (int)nlen > size) break;
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
        return NULL;
    }
    *out_size = pos;
    return out;
}

static int qsv_try_init_encoder(QsvEncoder* enc);

static void qsv_apply_ext_params(QsvEncoder* enc) {
    ZeroMemory(&enc->ext_copt, sizeof(enc->ext_copt));
    enc->ext_copt.Header.BufferId = MFX_EXTBUFF_CODING_OPTION;
    enc->ext_copt.Header.BufferSz = (mfxU16)sizeof(enc->ext_copt);
    enc->ext_copt.NalHrdConformance = MFX_CODINGOPTION_OFF;
    enc->ext_copt.PicTimingSEI = MFX_CODINGOPTION_OFF;
    enc->ext_copt.AUDelimiter = MFX_CODINGOPTION_OFF;

    ZeroMemory(&enc->ext_copt2, sizeof(enc->ext_copt2));
    enc->ext_copt2.Header.BufferId = MFX_EXTBUFF_CODING_OPTION2;
    enc->ext_copt2.Header.BufferSz = (mfxU16)sizeof(enc->ext_copt2);
    enc->ext_copt2.BitrateLimit = MFX_CODINGOPTION_ON;
    enc->ext_copt2.MBBRC = MFX_CODINGOPTION_ON;
    enc->ext_copt2.RepeatPPS = MFX_CODINGOPTION_OFF;
    enc->ext_copt2.MaxQPI = 38;
    enc->ext_copt2.MaxQPP = 40;
    {
        mfxU32 avg = qsv_avg_frame_bytes(enc);
        enc->ext_copt2.MaxFrameSize = avg * 2U;
    }

    ZeroMemory(&enc->ext_copt3, sizeof(enc->ext_copt3));
    enc->ext_copt3.Header.BufferId = MFX_EXTBUFF_CODING_OPTION3;
    enc->ext_copt3.Header.BufferSz = (mfxU16)sizeof(enc->ext_copt3);
    {
        mfxU32 avg = qsv_avg_frame_bytes(enc);
        enc->ext_copt3.MaxFrameSizeI = avg * 3U;
        enc->ext_copt3.MaxFrameSizeP = avg + avg / 2U;
        enc->ext_copt3.LowDelayBRC = MFX_CODINGOPTION_ON;
    }

    enc->ext_params[0] = (mfxExtBuffer*)&enc->ext_copt;
    enc->ext_params[1] = (mfxExtBuffer*)&enc->ext_copt2;
    enc->ext_params[2] = (mfxExtBuffer*)&enc->ext_copt3;
    enc->param.ExtParam = enc->ext_params;
    enc->param.NumExtParam = 3;
}

static void qsv_apply_rate_control(QsvEncoder* enc) {
    enc->param.mfx.RateControlMethod = MFX_RATECONTROL_CBR;
    enc->param.mfx.TargetKbps = (mfxU16)enc->bitrate_kbps;
    enc->param.mfx.MaxKbps = (mfxU16)enc->bitrate_kbps;
    enc->param.mfx.GopPicSize = (mfxU16)(enc->fps * 4);
    if (enc->param.mfx.GopPicSize < 40) {
        enc->param.mfx.GopPicSize = 40;
    }
    enc->param.mfx.GopRefDist = 1;
    enc->param.mfx.IdrInterval = 0;
    enc->param.mfx.BufferSizeInKB = (mfxU16)(enc->bitrate_kbps / (enc->fps > 0 ? enc->fps : 20) + 8);
    if (enc->param.mfx.BufferSizeInKB < 16) {
        enc->param.mfx.BufferSizeInKB = 16;
    }
    enc->param.mfx.InitialDelayInKB = 0;
    {
        mfxU32 avg = qsv_avg_frame_bytes(enc);
        enc->ext_copt2.MaxFrameSize = avg * 2U;
        enc->ext_copt3.MaxFrameSizeI = avg * 3U;
        enc->ext_copt3.MaxFrameSizeP = avg + avg / 2U;
    }
}

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

    /* System-memory NV12 (IOPattern below) — do not bind D3D11 via SetHandle; a failed
     * SetHandle leaves some Intel drivers with a corrupted session and AV on Query/Init. */

    mfxU16 aw = mfx_align16((mfxU16)width);
    mfxU16 ah = mfx_align16((mfxU16)height);

    ZeroMemory(&enc->param, sizeof(enc->param));
    enc->param.AsyncDepth = 1;
    enc->param.IOPattern = MFX_IOPATTERN_IN_SYSTEM_MEMORY;
    qsv_apply_ext_params(enc);
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
    enc->param.mfx.LowPower = 0;
    enc->param.mfx.CodecId = MFX_CODEC_AVC;
    enc->param.mfx.CodecProfile = MFX_PROFILE_AVC_BASELINE;
    enc->param.mfx.CodecLevel = 0;
    enc->param.mfx.TargetUsage = MFX_TARGETUSAGE_BEST_SPEED;
    enc->param.mfx.RateControlMethod = MFX_RATECONTROL_CBR;
    enc->param.mfx.TargetKbps = (mfxU16)enc->bitrate_kbps;
    enc->param.mfx.MaxKbps = (mfxU16)enc->bitrate_kbps;
    enc->param.mfx.BufferSizeInKB = (mfxU16)(enc->bitrate_kbps / (enc->fps > 0 ? enc->fps : 20) + 8);
    enc->param.mfx.GopPicSize = (mfxU16)(enc->fps * 2);
    enc->param.mfx.GopRefDist = 1;
    enc->param.mfx.IdrInterval = 1;
    enc->param.mfx.NumRefFrame = 1;
    enc->param.mfx.NumSlice = 1;

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
        {0, MFX_LEVEL_AVC_4, 2},
        {0, 41, 2},
        {0, 0, 2},
    };

    for (int i = 0; i < 3; i++) {
        enc->param.mfx.LowPower = (mfxU16)tries[i].low_power;
        enc->param.mfx.CodecLevel = tries[i].level;
        qsv_apply_ext_params(enc);
        qsv_apply_rate_control(enc);

        mfxVideoParam valid = enc->param;
        mfxStatus sts = enc->MFXVideoENCODE_Query(enc->session, &enc->param, &valid);
        if (sts < MFX_ERR_NONE) {
            qsv_set_errorf1(enc, "ENCODE_Query failed sts=%d", (int)sts);
            continue;
        }
        enc->param = valid;
        qsv_apply_ext_params(enc);
        qsv_apply_rate_control(enc);

        sts = enc->MFXVideoENCODE_Init(enc->session, &enc->param);
        if (sts >= MFX_ERR_NONE) {
            enc->encoder_inited = 1;
            qsv_set_error(enc, "");
            return 0;
        }
        qsv_set_errorf1(enc, "ENCODE_Init failed sts=%d", (int)sts);
        /* Do not ENCODE_Close after failed Init — crashes some Intel drivers. */
    }
    return -6;
}

int qsv_encode(QsvEncoder* enc, const uint8_t* nv12, int nv12_size, int pitch, int force_idr, uint8_t** out_data, int* out_size, int* out_keyframe) {
    if (!enc || !enc->session || !nv12 || !out_data || !out_size || !out_keyframe) return -1;
    if (pitch <= 0) return -1;
    if (nv12_size > 0 && nv12_size < (int)((size_t)pitch * (size_t)qsv_y_plane_height(enc) * 3 / 2)) {
        qsv_set_error(enc, "NV12 buffer too small");
        return -1;
    }

    mfxFrameSurface1 surface;
    ZeroMemory(&surface, sizeof(surface));
    surface.Info = enc->param.mfx.FrameInfo;
    surface.Data.Y = (mfxU8*)nv12;
    surface.Data.UV = (mfxU8*)(nv12 + (size_t)pitch * (size_t)qsv_y_plane_height(enc));
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

    mfxSyncPoint syncp = NULL;
    mfxStatus sts = MFX_ERR_NONE;
    for (int tries = 0; tries < 32; tries++) {
        bs.DataOffset = 0;
        bs.DataLength = 0;
        syncp = NULL;
        sts = enc->MFXVideoENCODE_EncodeFrameAsync(enc->session, pctrl, &surface, &bs, &syncp);
        if (sts == MFX_WRN_DEVICE_BUSY) {
            continue;
        }
        if (sts == MFX_ERR_NOT_ENOUGH_BUFFER || sts == MFX_ERR_MORE_BITSTREAM) {
            if (qsv_grow_bitstream(enc, &bs) != 0) {
                qsv_set_error(enc, "bitstream grow failed");
                return -3;
            }
            continue;
        }
        break;
    }

    if (sts == MFX_ERR_MORE_DATA) {
        enc->frame_index++;
        return 1;
    }
    if (sts < MFX_ERR_NONE) {
        qsv_set_errorf1(enc, "EncodeFrameAsync sts=%d", (int)sts);
        return -2;
    }

    if (syncp) {
        mfxStatus sync_sts = enc->MFXVideoCORE_SyncOperation(enc->session, syncp, 60000);
        if (sync_sts < MFX_ERR_NONE && sync_sts != MFX_ERR_ABORTED) {
            qsv_set_errorf1(enc, "SyncOperation sts=%d", (int)sync_sts);
            return -2;
        }
    }
    if (bs.DataLength == 0) {
        enc->frame_index++;
        return 1;
    }

    enc->priming = 0;
    int raw_size = (int)bs.DataLength;
    mfxU32 max_au = qsv_avg_frame_bytes(enc) * 32U;
    uint8_t* annexb = bitstream_to_annexb(bs.Data + bs.DataOffset, raw_size, &raw_size, max_au);
    if (!annexb) return 1;

    *out_size = raw_size;
    *out_data = annexb;
    *out_keyframe = (bs.FrameType & MFX_FRAMETYPE_IDR) ? 1 : 0;
    enc->frame_index++;
    return 0;
}

int qsv_set_bitrate(QsvEncoder* enc, int kbps) {
    if (!enc || !enc->session) return -1;
    enc->bitrate_kbps = kbps;
    enc->param.mfx.TargetKbps = (mfxU16)kbps;
    enc->param.mfx.MaxKbps = (mfxU16)kbps;
    enc->param.mfx.BufferSizeInKB = (mfxU16)(kbps / 8 + 1);
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
    if (enc->encoder_inited && enc->session && enc->MFXVideoENCODE_Close) {
        enc->MFXVideoENCODE_Close(enc->session);
        enc->encoder_inited = 0;
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
