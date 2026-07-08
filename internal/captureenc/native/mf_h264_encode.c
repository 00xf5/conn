#ifndef CONNECT_MF_H264_ENCODE_C
#define CONNECT_MF_H264_ENCODE_C

#include "mf_h264_encode.h"
#include "platform.h"

#include <mfapi.h>
#include <mfidl.h>
#include <mferror.h>
#include <wmcodecdsp.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

static void mf_set_frame_size(IMFMediaType* mt, UINT32 w, UINT32 h) {
    UINT64 v = ((UINT64)w << 32) | (UINT64)h;
    mt->lpVtbl->SetUINT64(mt, &MF_MT_FRAME_SIZE, v);
}

static void mf_set_frame_rate(IMFMediaType* mt, UINT32 num, UINT32 den) {
    UINT64 v = ((UINT64)num << 32) | (UINT64)den;
    mt->lpVtbl->SetUINT64(mt, &MF_MT_FRAME_RATE, v);
}

struct MfH264Enc {
    IMFTransform* xform;
    DWORD input_stream_id;
    DWORD output_stream_id;
    int width;
    int height;
    int fps;
    int bitrate_kbps;
    int hw;
    LONGLONG frame_index;
};

static int g_mf_started;
static int g_com_inited;

static int ensure_mf(void) {
    if (!g_com_inited) {
        HRESULT hr = CoInitializeEx(NULL, COINIT_MULTITHREADED);
        if (FAILED(hr) && hr != RPC_E_CHANGED_MODE) {
            return -1;
        }
        g_com_inited = 1;
    }
    if (!g_mf_started) {
        HRESULT hr = MFStartup(MF_VERSION, MFSTARTUP_LITE);
        if (FAILED(hr)) {
            return -2;
        }
        g_mf_started = 1;
    }
    return 0;
}

static int avcc_to_annex_b(const uint8_t* in, int in_len, uint8_t** out, int* out_len, int* keyframe) {
    *out = NULL;
    *out_len = 0;
    *keyframe = 0;
    if (!in || in_len < 4) {
        return -1;
    }
    int cap = in_len + 64;
    uint8_t* buf = (uint8_t*)malloc((size_t)cap);
    if (!buf) {
        return -2;
    }
    int pos = 0;
    int off = 0;
    while (off + 4 <= in_len) {
        uint32_t n = ((uint32_t)in[off] << 24) | ((uint32_t)in[off + 1] << 16) |
                     ((uint32_t)in[off + 2] << 8) | (uint32_t)in[off + 3];
        off += 4;
        if (n == 0 || off + (int)n > in_len) {
            break;
        }
        if (pos + 4 + (int)n > cap) {
            cap = pos + 4 + (int)n + 256;
            uint8_t* nb = (uint8_t*)realloc(buf, (size_t)cap);
            if (!nb) {
                free(buf);
                return -3;
            }
            buf = nb;
        }
        buf[pos++] = 0;
        buf[pos++] = 0;
        buf[pos++] = 0;
        buf[pos++] = 1;
        if (n >= 1) {
            uint8_t nal_type = in[off] & 0x1f;
            if (nal_type == 5) {
                *keyframe = 1;
            }
        }
        memcpy(buf + pos, in + off, n);
        pos += (int)n;
        off += (int)n;
    }
    if (pos == 0) {
        free(buf);
        return -4;
    }
    *out = buf;
    *out_len = pos;
    return 0;
}

static int set_codecapi_bitrate(IMFTransform* xform, int bitrate_kbps) {
    (void)xform;
    (void)bitrate_kbps;
    return 0;
}

static void mf_set_stride(IMFMediaType* mt, INT32 stride) {
    mt->lpVtbl->SetUINT32(mt, &MF_MT_DEFAULT_STRIDE, (UINT32)stride);
}

static int configure_types(MfH264Enc* enc) {
    IMFTransform* xform = enc->xform;
    HRESULT hr;

    UINT32 w = (UINT32)((enc->width + 15) & ~15);
    UINT32 h = (UINT32)enc->height;
    if (h & 1) h++;

    /* Many H.264 MFTs require output type before input. */
    int out_ok = 0;
    for (DWORD i = 0;; i++) {
        IMFMediaType* out_cand = NULL;
        hr = xform->lpVtbl->GetOutputAvailableType(xform, enc->output_stream_id, i, &out_cand);
        if (FAILED(hr) || !out_cand) {
            break;
        }

        hr = xform->lpVtbl->SetOutputType(xform, enc->output_stream_id, out_cand, 0);
        if (FAILED(hr)) {
            mf_set_frame_size(out_cand, w, h);
            mf_set_frame_rate(out_cand, (UINT32)enc->fps, 1);
            hr = xform->lpVtbl->SetOutputType(xform, enc->output_stream_id, out_cand, 0);
        }
        if (FAILED(hr)) {
            out_cand->lpVtbl->SetUINT32(out_cand, &MF_MT_AVG_BITRATE, (UINT32)(enc->bitrate_kbps * 1000));
            out_cand->lpVtbl->SetUINT32(out_cand, &MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
            out_cand->lpVtbl->SetUINT32(out_cand, &MF_MT_MPEG2_PROFILE, (UINT32)66); /* eAVEncH264VProfile_Base */
            hr = xform->lpVtbl->SetOutputType(xform, enc->output_stream_id, out_cand, 0);
        }
        out_cand->lpVtbl->Release(out_cand);
        if (SUCCEEDED(hr)) {
            out_ok = 1;
            break;
        }
    }
    if (!out_ok) {
        return -13;
    }

    IMFMediaType* in_type = NULL;
    hr = xform->lpVtbl->GetInputAvailableType(xform, enc->input_stream_id, 0, &in_type);
    if (FAILED(hr) || !in_type) {
        hr = MFCreateMediaType(&in_type);
        if (FAILED(hr) || !in_type) {
            return -11;
        }
        in_type->lpVtbl->SetGUID(in_type, &MF_MT_MAJOR_TYPE, &MFMediaType_Video);
        in_type->lpVtbl->SetGUID(in_type, &MF_MT_SUBTYPE, &MFVideoFormat_NV12);
    }
    mf_set_frame_size(in_type, w, h);
    mf_set_frame_rate(in_type, (UINT32)enc->fps, 1);
    mf_set_stride(in_type, (INT32)w);
    in_type->lpVtbl->SetUINT32(in_type, &MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
    hr = xform->lpVtbl->SetInputType(xform, enc->input_stream_id, in_type, 0);
    in_type->lpVtbl->Release(in_type);
    if (FAILED(hr)) {
        return -11;
    }

    enc->width = (int)w;
    enc->height = (int)h;
    set_codecapi_bitrate(xform, enc->bitrate_kbps);
    return 0;
}

static int create_transform(MfH264Enc* enc) {
    HRESULT hr;
    IMFTransform* xform = NULL;
    MFT_REGISTER_TYPE_INFO in_info = {MFMediaType_Video, MFVideoFormat_NV12};
    MFT_REGISTER_TYPE_INFO out_info = {MFMediaType_Video, MFVideoFormat_H264};
    IMFActivate** activates = NULL;
    UINT32 count = 0;
    UINT32 flag_sets[] = {
        MFT_ENUM_FLAG_HARDWARE | MFT_ENUM_FLAG_SORTANDFILTER,
        MFT_ENUM_FLAG_SYNCMFT | MFT_ENUM_FLAG_SORTANDFILTER,
    };

    for (int f = 0; f < 2 && !xform; f++) {
        hr = MFTEnumEx(MFT_CATEGORY_VIDEO_ENCODER, flag_sets[f], &in_info, &out_info, &activates, &count);
        if (FAILED(hr) || count == 0) {
            continue;
        }
        for (UINT32 i = 0; i < count; i++) {
            hr = activates[i]->lpVtbl->ActivateObject(activates[i], &IID_IMFTransform, (void**)&xform);
            if (SUCCEEDED(hr) && xform) {
                enc->hw = (flag_sets[f] & MFT_ENUM_FLAG_HARDWARE) ? 1 : 0;
                break;
            }
            xform = NULL;
        }
        for (UINT32 i = 0; i < count; i++) {
            activates[i]->lpVtbl->Release(activates[i]);
        }
        CoTaskMemFree(activates);
        activates = NULL;
        count = 0;
    }
    if (!xform) {
        return -21;
    }

    enc->xform = xform;
    enc->input_stream_id = 0;
    enc->output_stream_id = 0;
    DWORD in_ids[1] = {0};
    DWORD out_ids[1] = {0};
    hr = xform->lpVtbl->GetStreamIDs(xform, 1, in_ids, 1, out_ids);
    if (SUCCEEDED(hr)) {
        enc->input_stream_id = in_ids[0];
        enc->output_stream_id = out_ids[0];
    }

    int rc = configure_types(enc);
    if (rc != 0) {
        xform->lpVtbl->Release(xform);
        enc->xform = NULL;
        return rc;
    }
    xform->lpVtbl->ProcessMessage(xform, MFT_MESSAGE_COMMAND_FLUSH, 0);
    xform->lpVtbl->ProcessMessage(xform, MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);
    xform->lpVtbl->ProcessMessage(xform, MFT_MESSAGE_NOTIFY_START_OF_STREAM, enc->input_stream_id);
    return 0;
}

int mf_h264_enc_init(MfH264Enc** out, int width, int height, int fps, int bitrate_kbps,
                     char* name_out, int name_cap) {
    if (!out || width <= 0 || height <= 0) {
        return -1;
    }
    int mf_rc = ensure_mf();
    if (mf_rc != 0) {
        return mf_rc;
    }
    if (fps <= 0) fps = 20;
    if (bitrate_kbps <= 0) bitrate_kbps = 2000;

    MfH264Enc* enc = (MfH264Enc*)calloc(1, sizeof(MfH264Enc));
    if (!enc) {
        return -3;
    }
    enc->width = width;
    enc->height = height;
    enc->fps = fps;
    enc->bitrate_kbps = bitrate_kbps;

    int rc = create_transform(enc);
    if (rc != 0) {
        free(enc);
        return rc;
    }

    if (name_out && name_cap > 0) {
        snprintf(name_out, (size_t)name_cap, "%s", enc->hw ? "mf-h264-hw" : "mf-h264");
    }
    *out = enc;
    return 0;
}

static int create_nv12_sample(MfH264Enc* enc, const uint8_t* nv12, int pitch, IMFSample** out_sample) {
    int h = enc->height;
    int w = enc->width;
    int uv_h = h / 2;
    int y_size = pitch * h;
    int uv_size = pitch * uv_h;
    int total = y_size + uv_size;

    IMFMediaBuffer* buf = NULL;
  HRESULT hr = MFCreateMemoryBuffer((DWORD)total, &buf);
    if (FAILED(hr)) return -30;

    BYTE* dst = NULL;
    hr = buf->lpVtbl->Lock(buf, &dst, NULL, NULL);
    if (FAILED(hr)) {
        buf->lpVtbl->Release(buf);
        return -31;
    }
    const uint8_t* y = nv12;
    const uint8_t* uv = nv12 + pitch * h;
    for (int row = 0; row < h; row++) {
        memcpy(dst + row * pitch, y + row * pitch, (size_t)w);
    }
    uint8_t* dst_uv = dst + pitch * h;
    for (int row = 0; row < uv_h; row++) {
        memcpy(dst_uv + row * pitch, uv + row * pitch, (size_t)w);
    }
    buf->lpVtbl->Unlock(buf);
    buf->lpVtbl->SetCurrentLength(buf, (DWORD)total);

    IMFSample* sample = NULL;
    hr = MFCreateSample(&sample);
    if (FAILED(hr)) {
        buf->lpVtbl->Release(buf);
        return -32;
    }
    sample->lpVtbl->AddBuffer(sample, buf);
    buf->lpVtbl->Release(buf);

    LONGLONG rt = enc->frame_index * 10000000LL / enc->fps;
    sample->lpVtbl->SetSampleTime(sample, rt);
    sample->lpVtbl->SetSampleDuration(sample, 10000000LL / enc->fps);
    enc->frame_index++;

    *out_sample = sample;
    return 0;
}

int mf_h264_enc_encode(MfH264Enc* enc, const uint8_t* nv12, int pitch, int width, int height,
                       MfH264Packet* pkt) {
    if (!enc || !enc->xform || !nv12 || !pkt) {
        return -1;
    }
    memset(pkt, 0, sizeof(*pkt));

    IMFSample* in_sample = NULL;
    int rc = create_nv12_sample(enc, nv12, pitch, &in_sample);
    if (rc != 0) {
        return rc;
    }

    HRESULT hr = enc->xform->lpVtbl->ProcessInput(enc->xform, enc->input_stream_id, in_sample, 0);
    in_sample->lpVtbl->Release(in_sample);
    if (FAILED(hr)) {
        return -40;
    }

    for (int attempt = 0; attempt < 4; attempt++) {
        MFT_OUTPUT_DATA_BUFFER out_buf;
        DWORD status = 0;
        memset(&out_buf, 0, sizeof(out_buf));
        out_buf.dwStreamID = enc->output_stream_id;

        hr = enc->xform->lpVtbl->ProcessOutput(enc->xform, 0, 1, &out_buf, &status);
        if (hr == MF_E_TRANSFORM_NEED_MORE_INPUT) {
            return 1;
        }
        if (hr == MF_E_TRANSFORM_STREAM_CHANGE) {
            if (out_buf.pSample) out_buf.pSample->lpVtbl->Release(out_buf.pSample);
            if (out_buf.pEvents) out_buf.pEvents->lpVtbl->Release(out_buf.pEvents);
            if (configure_types(enc) != 0) {
                return -45;
            }
            continue;
        }
        if (FAILED(hr)) {
            if (out_buf.pSample) out_buf.pSample->lpVtbl->Release(out_buf.pSample);
            if (out_buf.pEvents) out_buf.pEvents->lpVtbl->Release(out_buf.pEvents);
            return -41;
        }

        IMFSample* out_sample = out_buf.pSample;
        if (!out_sample) {
            return 1;
        }

        IMFMediaBuffer* buf = NULL;
        hr = out_sample->lpVtbl->ConvertToContiguousBuffer(out_sample, &buf);
        out_sample->lpVtbl->Release(out_sample);
        if (FAILED(hr)) {
            return -42;
        }

        BYTE* data = NULL;
        DWORD max_len = 0, cur_len = 0;
        hr = buf->lpVtbl->Lock(buf, &data, &max_len, &cur_len);
        if (FAILED(hr)) {
            buf->lpVtbl->Release(buf);
            return -43;
        }

        int key = 0;
        rc = avcc_to_annex_b(data, (int)cur_len, &pkt->data, &pkt->size, &key);
        buf->lpVtbl->Unlock(buf);
        buf->lpVtbl->Release(buf);
        if (rc != 0) {
            return rc;
        }
        pkt->keyframe = key;
        return 0;
    }
    return 1;
}

void mf_h264_enc_release_packet(MfH264Packet* pkt) {
    if (pkt && pkt->data) {
        free(pkt->data);
        pkt->data = NULL;
        pkt->size = 0;
    }
}

int mf_h264_enc_set_bitrate(MfH264Enc* enc, int bitrate_kbps) {
    if (!enc || !enc->xform || bitrate_kbps <= 0) {
        return -1;
    }
    enc->bitrate_kbps = bitrate_kbps;
    return set_codecapi_bitrate(enc->xform, bitrate_kbps);
}

void mf_h264_enc_shutdown(MfH264Enc* enc) {
    if (!enc) return;
    if (enc->xform) {
        enc->xform->lpVtbl->Release(enc->xform);
        enc->xform = NULL;
    }
    free(enc);
}

#endif
