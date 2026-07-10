#include "bridge.h"

#include "dxgi_capture.h"
#include "nvenc_dyn.h"
#include "qsv_encode.h"
#include "mf_h264_encode.h"

#include <stdlib.h>
#include <string.h>
#include <stdio.h>

typedef enum {
    ENC_BACKEND_NONE = 0,
    ENC_BACKEND_NVENC = 1,
    ENC_BACKEND_MF = 2,
    ENC_BACKEND_QSV = 3,
} EncBackend;

typedef struct {
    CaptureEncConfig cfg;
    DxgiCapture dxgi;
    NvencEncoder nvenc;
    QsvEncoder qsv;
    MfH264Enc* mf;
    EncBackend backend;
    char encoder_name[64];
    uint64_t qpc_freq;
    uint64_t last_frame_qpc;
    uint64_t frame_interval_qpc;
    uint64_t last_reinit_qpc;
    int need_reinit;
    int has_frame_cache;
    int empty_reads;
} CaptureEncState;

static void update_pacing(CaptureEncState* st) {
    st->qpc_freq = platform_qpc_freq();
    st->frame_interval_qpc = st->qpc_freq / (uint64_t)(st->cfg.fps > 0 ? st->cfg.fps : 30);
    st->last_frame_qpc = 0;
}

static void shutdown_encoders(CaptureEncState* st) {
    nvenc_shutdown(&st->nvenc);
    qsv_shutdown(&st->qsv);
    if (st->mf) {
        mf_h264_enc_shutdown(st->mf);
        st->mf = NULL;
    }
    st->backend = ENC_BACKEND_NONE;
}

static int nvenc_dll_present(void) {
    HMODULE m = GetModuleHandleA("nvEncodeAPI64.dll");
    if (!m) {
        m = LoadLibraryA("nvEncodeAPI64.dll");
        if (m) {
            FreeLibrary(m);
            return 1;
        }
        return 0;
    }
    return 1;
}

static int reinit(CaptureEncState* st) {
    dxgi_capture_shutdown(&st->dxgi);
    shutdown_encoders(st);

    if (dxgi_capture_init(&st->dxgi, st->cfg.monitor_index, st->cfg.width, st->cfg.height, st->cfg.fps) != 0) {
        return -20;
    }

    if (nvenc_dll_present() &&
        nvenc_init(&st->nvenc, st->dxgi.device, st->dxgi.width, st->dxgi.height, st->cfg.fps, st->cfg.bitrate_kbps) == 0 &&
        nvenc_register_texture(&st->nvenc, st->dxgi.nv12) == 0) {
        st->backend = ENC_BACKEND_NVENC;
        snprintf(st->encoder_name, sizeof(st->encoder_name), "%s", st->nvenc.name);
        st->nvenc.force_idr = 1;
    } else {
        nvenc_shutdown(&st->nvenc);
        int qrc = qsv_init(&st->qsv, st->dxgi.device, st->dxgi.width, st->dxgi.height, st->cfg.fps, st->cfg.bitrate_kbps);
        if (qrc == 0) {
            st->backend = ENC_BACKEND_QSV;
            snprintf(st->encoder_name, sizeof(st->encoder_name), "%s", st->qsv.name);
            st->qsv.force_idr = 1;
        } else {
            fprintf(stderr, "connect: qsv_init=%d (%s)\n", qrc, qsv_last_error(&st->qsv));
            fflush(stderr);
            qsv_shutdown(&st->qsv);
            char mf_name[64] = {0};
            int mrc = mf_h264_enc_init(&st->mf, st->dxgi.width, st->dxgi.height, st->cfg.fps, st->cfg.bitrate_kbps,
                                       mf_name, (int)sizeof(mf_name));
            if (mrc == 0) {
                st->backend = ENC_BACKEND_MF;
                snprintf(st->encoder_name, sizeof(st->encoder_name), "%s", mf_name);
            } else {
                fprintf(stderr, "connect: mf_h264_enc_init=%d\n", mrc);
                fflush(stderr);
                if (st->mf) {
                    mf_h264_enc_shutdown(st->mf);
                    st->mf = NULL;
                }
                dxgi_capture_shutdown(&st->dxgi);
                return qrc != 0 ? qrc : mrc;
            }
        }
    }

    st->need_reinit = 0;
    st->has_frame_cache = 0;
    st->empty_reads = 0;
    update_pacing(st);
    return 0;
}

int captureenc_init(const CaptureEncConfig* cfg, CaptureEncHandle* out) {
    if (!cfg || !out) {
        return -1;
    }
    CaptureEncState* st = (CaptureEncState*)calloc(1, sizeof(CaptureEncState));
    if (!st) {
        return -2;
    }
    st->cfg = *cfg;
    if (st->cfg.fps <= 0) st->cfg.fps = 30;
    if (st->cfg.bitrate_kbps <= 0) st->cfg.bitrate_kbps = 4000;

    int rc = reinit(st);
    if (rc != 0) {
        free(st);
        return rc;
    }
    *out = (CaptureEncHandle)st;
    return 0;
}

static void captureenc_finish_frame(CaptureEncState* st, CaptureEncFrame* out, uint8_t* data, int size, int keyframe, uint64_t now) {
    out->data = data;
    out->size = size;
    out->keyframe = keyframe;
    out->timestamp_us = platform_qpc_to_us(now, st->qpc_freq);
    st->last_frame_qpc = now;
    st->empty_reads = 0;
}

static int captureenc_acquire_nv12(CaptureEncState* st) {
    ID3D11Texture2D* bgra = NULL;
    int acq = dxgi_capture_acquire(&st->dxgi, &bgra);
    if (acq == 1) {
        return st->has_frame_cache ? 0 : 1;
    }
    if (acq == -2) {
        st->need_reinit = 1;
        return 1;
    }
    if (acq != 0) {
        return -40;
    }
    if (dxgi_capture_convert_nv12(&st->dxgi, bgra) != 0) {
        dxgi_capture_release(&st->dxgi);
        return -41;
    }
    dxgi_capture_release(&st->dxgi);
    st->has_frame_cache = 1;
    return 0;
}

static int captureenc_encode_qsv(CaptureEncState* st, uint64_t now, CaptureEncFrame* out) {
    uint8_t* data = NULL;
    int size = 0;
    int keyframe = 0;

    if (qsv_drain(&st->qsv, &data, &size, &keyframe) == 0 && size > 0 && data) {
        captureenc_finish_frame(st, out, data, size, keyframe, now);
        return 0;
    }
    if (data) {
        free(data);
        data = NULL;
    }

    int cap = captureenc_acquire_nv12(st);
    if (cap == 1) {
        return 1;
    }
    if (cap != 0) {
        return cap;
    }

    if (dxgi_capture_map_nv12(&st->dxgi) != 0) {
        return -43;
    }
    int pitch = 0;
    const uint8_t* nv12 = dxgi_capture_nv12_bytes(&st->dxgi, &pitch);
    int force = (st->qsv.frame_index == 0) ? 1 : 0;

    for (int feed = 0; feed < 32; feed++) {
        int rc = qsv_encode(&st->qsv, nv12, (int)st->dxgi.nv12_cpu_size, pitch, force, &data, &size, &keyframe);
        force = 0;
        if (rc == 0 && size > 0 && data) {
            captureenc_finish_frame(st, out, data, size, keyframe, now);
            return 0;
        }
        if (data) {
            free(data);
            data = NULL;
        }
        if (rc == 1) {
            continue;
        }
        if (rc == -2 || rc == -3) {
            return 1;
        }
        break;
    }
    return 1;
}

int captureenc_recover(CaptureEncHandle handle) {
    if (!handle) {
        return -1;
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    st->need_reinit = 1;
    st->has_frame_cache = 0;
    st->empty_reads = 0;
    return reinit(st);
}

int captureenc_read_frame(CaptureEncHandle handle, CaptureEncFrame* out) {
    if (!handle || !out) {
        return -1;
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    ZeroMemory(out, sizeof(*out));

    if (st->need_reinit) {
        uint64_t now_qpc = platform_qpc_now();
        /* While locked, DuplicateOutput fails — throttle retries to ~2/sec (not every frame). */
        if (st->qpc_freq == 0) {
            st->qpc_freq = platform_qpc_freq();
        }
        if (st->last_reinit_qpc != 0 && st->qpc_freq != 0) {
            uint64_t gap = st->qpc_freq / 2; /* 500ms */
            if (now_qpc - st->last_reinit_qpc < gap) {
                return 1;
            }
        }
        st->last_reinit_qpc = now_qpc;
        if (reinit(st) != 0) {
            st->need_reinit = 1;
            st->has_frame_cache = 0;
            return 1;
        }
        st->need_reinit = 0;
        st->empty_reads = 0;
        st->has_frame_cache = 0;
        update_pacing(st);
    }

    uint64_t now = platform_qpc_now();

    int rc = -42;

    if (st->backend == ENC_BACKEND_QSV) {
        rc = captureenc_encode_qsv(st, now, out);
        if (rc == 0) {
            return 0;
        }
        if (rc == 1) {
            st->empty_reads++;
            if (st->empty_reads > st->cfg.fps * 3) {
                st->need_reinit = 1;
                st->empty_reads = 0;
                fprintf(stderr, "connect: pipeline recover after %d empty reads\n", st->cfg.fps * 3);
                fflush(stderr);
            }
            return 1;
        }
        return rc;
    }

    uint8_t* data = NULL;
    int size = 0;
    int keyframe = 0;

    int cap = captureenc_acquire_nv12(st);
    if (cap == 1) {
        st->empty_reads++;
        if (st->empty_reads > st->cfg.fps * 3) {
            st->need_reinit = 1;
            st->empty_reads = 0;
        }
        return 1;
    }
    if (cap != 0) {
        return cap;
    }

    if (st->backend == ENC_BACKEND_NVENC) {
        int force = (st->nvenc.frame_index == 0) ? 1 : 0;
        rc = nvenc_encode(&st->nvenc, force, &data, &size, &keyframe);
    } else if (st->backend == ENC_BACKEND_MF) {
        if (dxgi_capture_map_nv12(&st->dxgi) != 0) {
            return -43;
        }
        int pitch = 0;
        const uint8_t* nv12 = dxgi_capture_nv12_bytes(&st->dxgi, &pitch);
        MfH264Packet pkt;
        rc = mf_h264_enc_encode(st->mf, nv12, pitch, st->dxgi.width, st->dxgi.height, &pkt);
        if (rc == 0) {
            data = pkt.data;
            size = pkt.size;
            keyframe = pkt.keyframe;
        }
    } else {
        return -42;
    }

    if (rc == 1) {
        st->empty_reads++;
        return 1;
    }
    if (rc != 0) {
        if (rc == -2 || rc == -3 || rc == -40 || rc == -41 || rc == -43) {
            fprintf(stderr, "connect: encode drop rc=%d (%s)\n", rc,
                    st->backend == ENC_BACKEND_QSV ? qsv_last_error(&st->qsv) : "mf");
            fflush(stderr);
            return 1;
        }
        return rc;
    }
    if (size <= 0 || !data) {
        free(data);
        st->empty_reads++;
        return 1;
    }

    captureenc_finish_frame(st, out, data, size, keyframe, now);
    return 0;
}

void captureenc_release_frame(CaptureEncHandle handle, CaptureEncFrame* frame) {
    (void)handle;
    if (frame && frame->data) {
        free(frame->data);
        frame->data = NULL;
        frame->size = 0;
    }
}

int captureenc_set_bitrate(CaptureEncHandle handle, int kbps) {
    if (!handle) {
        return -1;
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    st->cfg.bitrate_kbps = kbps;
    if (st->backend == ENC_BACKEND_NVENC) {
        return nvenc_set_bitrate(&st->nvenc, kbps);
    }
    if (st->backend == ENC_BACKEND_MF) {
        return mf_h264_enc_set_bitrate(st->mf, kbps);
    }
    if (st->backend == ENC_BACKEND_QSV) {
        return qsv_set_bitrate(&st->qsv, kbps);
    }
    return 0;
}

int captureenc_request_keyframe(CaptureEncHandle handle) {
    if (!handle) {
        return -1;
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    if (st->backend == ENC_BACKEND_NVENC) {
        st->nvenc.force_idr = 1;
        return 0;
    }
    if (st->backend == ENC_BACKEND_QSV) {
        st->qsv.force_idr = 1;
        return 0;
    }
    return 0;
}

const char* captureenc_encoder_name(CaptureEncHandle handle) {
    if (!handle) {
        return "none";
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    return st->encoder_name;
}

void captureenc_capture_size(CaptureEncHandle handle, int* width, int* height) {
    if (width) *width = 0;
    if (height) *height = 0;
    if (!handle) {
        return;
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    if (width) *width = st->dxgi.width;
    if (height) *height = st->dxgi.height;
}

void captureenc_shutdown(CaptureEncHandle handle) {
    if (!handle) {
        return;
    }
    CaptureEncState* st = (CaptureEncState*)handle;
    shutdown_encoders(st);
    dxgi_capture_shutdown(&st->dxgi);
    free(st);
}
