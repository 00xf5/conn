#ifndef CONNECT_MF_H264_ENCODE_H
#define CONNECT_MF_H264_ENCODE_H

#include <stdint.h>

typedef struct MfH264Enc MfH264Enc;

typedef struct {
    uint8_t* data;
    int size;
    int keyframe;
} MfH264Packet;

/* Returns 0 on success. name_out is e.g. "mf-h264" or "mf-h264-hw". */
int mf_h264_enc_init(MfH264Enc** out, int width, int height, int fps, int bitrate_kbps,
                     char* name_out, int name_cap);

/* Encode one NV12 frame (full width x height). Returns 0 on packet, 1 if no output yet, <0 on error. */
int mf_h264_enc_encode(MfH264Enc* enc, const uint8_t* nv12, int pitch, int width, int height,
                       MfH264Packet* pkt);

void mf_h264_enc_release_packet(MfH264Packet* pkt);

int mf_h264_enc_set_bitrate(MfH264Enc* enc, int bitrate_kbps);

void mf_h264_enc_shutdown(MfH264Enc* enc);

#endif
