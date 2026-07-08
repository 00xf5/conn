#ifndef CONNECT_QSV_ENCODE_H
#define CONNECT_QSV_ENCODE_H

#include "mfx_minimal.h"
#include "platform.h"
#include <objbase.h>

struct ID3D11Device;

typedef struct {
    HMODULE lib;
    mfxSession session;
    mfxVideoParam param;
    mfxBitstream bitstream;
    mfxExtCodingOption ext_copt;
    mfxExtCodingOption2 ext_copt2;
    mfxExtCodingOption3 ext_copt3;
    mfxExtBuffer* ext_params[3];
    int width;
    int height;
    int fps;
    int bitrate_kbps;
    int frame_index;
    int force_idr;
    int priming;
    int encoder_inited;
    char name[64];
    char last_error[128];

    MFXInit_t MFXInit;
    MFXClose_t MFXClose;
    MFXVideoCORE_SetHandle_t MFXVideoCORE_SetHandle;
    MFXVideoENCODE_Query_t MFXVideoENCODE_Query;
    MFXVideoENCODE_Init_t MFXVideoENCODE_Init;
    MFXVideoENCODE_Close_t MFXVideoENCODE_Close;
    MFXVideoENCODE_Reset_t MFXVideoENCODE_Reset;
    MFXVideoENCODE_EncodeFrameAsync_t MFXVideoENCODE_EncodeFrameAsync;
    MFXVideoCORE_SyncOperation_t MFXVideoCORE_SyncOperation;
} QsvEncoder;

int qsv_init(QsvEncoder* enc, struct ID3D11Device* d3d_device, int width, int height, int fps, int bitrate_kbps);
int qsv_encode(QsvEncoder* enc, const uint8_t* nv12, int nv12_size, int pitch, int force_idr, uint8_t** out_data, int* out_size, int* out_keyframe);
int qsv_set_bitrate(QsvEncoder* enc, int kbps);
const char* qsv_last_error(QsvEncoder* enc);
void qsv_shutdown(QsvEncoder* enc);

#endif
