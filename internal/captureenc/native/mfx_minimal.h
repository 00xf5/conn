#ifndef CONNECT_MFX_MINIMAL_H
#define CONNECT_MFX_MINIMAL_H

#include <stdint.h>

typedef int32_t mfxStatus;
typedef int32_t mfxIMPL;
typedef uint8_t mfxU8;
typedef uint16_t mfxU16;
typedef uint32_t mfxU32;
typedef uint64_t mfxU64;
typedef void* mfxSession;
typedef void* mfxMemId;

#define MFX_ERR_NONE 0
#define MFX_ERR_NULL_PTR (-2)
#define MFX_ERR_MORE_DATA (-10)
#define MFX_ERR_NOT_ENOUGH_BUFFER (-5)
#define MFX_ERR_UNDEFINED_BEHAVIOR (-16)
#define MFX_WRN_DEVICE_BUSY 1

#define MFX_IMPL_HARDWARE 0x0002
#define MFX_IMPL_HARDWARE_ANY 0x0004
#define MFX_IMPL_HARDWARE2 0x0005
#define MFX_IMPL_VIA_D3D11 0x0300
#define MFX_HANDLE_D3D11_DEVICE 3

#define MFX_MAKEFOURCC(A,B,C,D) ((((int)(A)))+(((int)(B))<<8)+(((int)(C))<<16)+(((int)(D))<<24))
#define MFX_CODEC_AVC MFX_MAKEFOURCC('A','V','C',' ')
#define MFX_FOURCC_NV12 MFX_MAKEFOURCC('N','V','1','2')
#define MFX_CHROMAFORMAT_YUV420 1
#define MFX_PICSTRUCT_PROGRESSIVE 0x01
#define MFX_IOPATTERN_IN_SYSTEM_MEMORY 0x02
#define MFX_TARGETUSAGE_BEST_SPEED 7
#define MFX_RATECONTROL_CBR 1
#define MFX_PROFILE_AVC_BASELINE 1
#define MFX_LEVEL_AVC_40 40
#define MFX_ERR_INVALID_VIDEO_PARAM (-15)
#define MFX_FRAMETYPE_I 0x0001
#define MFX_FRAMETYPE_IDR 0x0004
#define MFX_FRAMETYPE_REF 0x0040

#pragma pack(push, 4)
typedef struct { mfxU16 Minor; mfxU16 Major; } mfxVersion;

typedef struct {
    mfxU16 TemporalId;
    mfxU16 PriorityId;
    union {
        struct { mfxU16 DependencyId; mfxU16 QualityId; };
        struct { mfxU16 ViewId; };
    };
} mfxFrameId;

typedef struct {
    mfxU32 reserved[4];
    mfxU16 reserved4;
    mfxU16 BitDepthLuma;
    mfxU16 BitDepthChroma;
    mfxU16 Shift;
    mfxFrameId FrameId;
    mfxU32 FourCC;
    union {
        struct { mfxU16 Width; mfxU16 Height; mfxU16 CropX; mfxU16 CropY; mfxU16 CropW; mfxU16 CropH; };
        struct { mfxU64 BufferSize; mfxU32 reserved5; };
    };
    mfxU32 FrameRateExtN;
    mfxU32 FrameRateExtD;
    mfxU16 reserved3;
    mfxU16 AspectRatioW;
    mfxU16 AspectRatioH;
    mfxU16 PicStruct;
    mfxU16 ChromaFormat;
    mfxU16 reserved2;
} mfxFrameInfo;

typedef struct {
    mfxU32 reserved[7];
    mfxU16 LowPower;
    mfxU16 BRCParamMultiplier;
    mfxFrameInfo FrameInfo;
    mfxU32 CodecId;
    mfxU16 CodecProfile;
    mfxU16 CodecLevel;
    mfxU16 NumThread;
    union {
        struct {
            mfxU16 TargetUsage;
            mfxU16 GopPicSize;
            mfxU16 GopRefDist;
            mfxU16 GopOptFlag;
            mfxU16 IdrInterval;
            mfxU16 RateControlMethod;
            mfxU16 InitialDelayInKB;
            mfxU16 BufferSizeInKB;
            mfxU16 TargetKbps;
            mfxU16 MaxKbps;
            mfxU16 NumSlice;
            mfxU16 NumRefFrame;
            mfxU16 EncodedOrder;
        } enc;
        struct {
            mfxU16 DecodedOrder;
            mfxU16 ExtendedPicStruct;
            mfxU16 TimeStampCalc;
            mfxU16 SliceGroupsPresent;
            mfxU16 MaxDecFrameBuffering;
            mfxU16 EnableReallocRequest;
            mfxU16 reserved2[7];
        } dec;
    } u;
} mfxInfoMFX;

typedef struct { mfxU16 FrameType; mfxU16 reserved[7]; } mfxEncodeCtrl;
#pragma pack(pop)

#pragma pack(push, 8)
typedef struct {
    union { void** ExtParam; mfxU64 reserved2; };
    mfxU16 NumExtParam;
    mfxU16 reserved[9];
    mfxU16 MemType;
    mfxU16 PitchHigh;
    mfxU64 TimeStamp;
    mfxU32 FrameOrder;
    mfxU16 Locked;
    union { mfxU16 Pitch; mfxU16 PitchLow; };
    union { mfxU8* Y; mfxU16* Y16; mfxU8* R; };
    union { mfxU8* UV; mfxU8* VU; mfxU8* CbCr; mfxU8* CrCb; mfxU8* Cb; mfxU8* U; mfxU16* U16; mfxU8* G; };
    union { mfxU8* Cr; mfxU8* V; mfxU16* V16; mfxU8* B; };
    mfxU8* A;
    mfxMemId MemId;
    mfxU16 Corrupted;
    mfxU16 DataFlag;
} mfxFrameData;

typedef struct { mfxU32 reserved[4]; mfxFrameInfo Info; mfxFrameData Data; } mfxFrameSurface1;

typedef struct {
    mfxU32 AllocId;
    mfxU32 reserved[2];
    mfxU16 reserved3;
    mfxU16 AsyncDepth;
    mfxInfoMFX mfx;
    mfxU16 Protected;
    mfxU16 IOPattern;
    void** ExtParam;
    mfxU16 NumExtParam;
    mfxU16 reserved2;
} mfxVideoParam;

typedef struct {
    union { struct { mfxU32 DataOffset; mfxU32 DataLength; }; mfxU64 reserved1; };
    mfxU32 MaxLength;
    mfxU32 TimeStampLow;
    mfxU32 TimeStampHigh;
    mfxU32 FrameType;
    mfxU32 DataFlag;
    mfxU8* Data;
} mfxBitstream;
#pragma pack(pop)

typedef mfxStatus (*MFXInit_t)(mfxIMPL impl, mfxVersion* ver, mfxSession* session);
typedef mfxStatus (*MFXClose_t)(mfxSession session);
typedef mfxStatus (*MFXVideoCORE_SetHandle_t)(mfxSession session, mfxU32 type, mfxMemId hdl);
typedef mfxStatus (*MFXVideoENCODE_Query_t)(mfxSession session, mfxVideoParam* in, mfxVideoParam* out);
typedef mfxStatus (*MFXVideoENCODE_Init_t)(mfxSession session, mfxVideoParam* par);
typedef mfxStatus (*MFXVideoENCODE_Close_t)(mfxSession session);
typedef mfxStatus (*MFXVideoENCODE_Reset_t)(mfxSession session, mfxVideoParam* par);
typedef mfxStatus (*MFXVideoENCODE_EncodeFrameAsync_t)(mfxSession session, mfxEncodeCtrl* ctrl, mfxFrameSurface1* surface, mfxBitstream* bs, void** syncp);
typedef mfxStatus (*MFXVideoCORE_SyncOperation_t)(mfxSession session, void* syncp, mfxU32 wait);

static inline mfxU16 mfx_align16(mfxU16 v) { return (mfxU16)((v + 15) & ~15); }

#endif
