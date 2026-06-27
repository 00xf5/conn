#ifndef CONNECT_NVENC_MINIMAL_H
#define CONNECT_NVENC_MINIMAL_H

#include <stdint.h>
#include <guiddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* NV_ENC_INPUT_PTR;
typedef void* NV_ENC_OUTPUT_PTR;

typedef int32_t NVENCSTATUS;
#define NV_ENC_SUCCESS 0

#define NVENCAPI __stdcall

typedef struct _NVENC_EXTERNAL_ME_HINT {
    uint32_t mvx;
    uint32_t mvy;
    uint32_t refidx;
    uint32_t dir;
    uint32_t lifetime;
} NVENC_EXTERNAL_ME_HINT;

typedef enum {
    NV_ENC_DEVICE_TYPE_DIRECTX = 0x0,
} NV_ENC_DEVICE_TYPE;

typedef enum {
    NV_ENC_BUFFER_FORMAT_NV12 = 0x1,
} NV_ENC_BUFFER_FORMAT;

typedef enum {
    NV_ENC_PIC_TYPE_IDR = 3,
} NV_ENC_PIC_TYPE;

typedef enum {
    NV_ENC_PARAMS_RC_CBR = 0x2,
} NV_ENC_PARAMS_RC_MODE;

typedef enum {
    NV_ENC_TUNING_INFO_LOW_LATENCY = 3,
} NV_ENC_TUNING_INFO;

typedef enum {
    NV_ENC_CODEC_H264_GUID_DEFINED = 1,
} NV_ENC_CODEC;

static const GUID NV_ENC_CODEC_H264_GUID =
    {0x6bc82762, 0x4e63, 0x4ca4, {0xaa, 0x85, 0x1e, 0x50, 0xf3, 0x21, 0xf6, 0xbf}};
static const GUID NV_ENC_PRESET_P3_GUID =
    {0xf082046b, 0x0c68, 0x436a, {0xb0, 0x2b, 0x9b, 0x9b, 0x6d, 0x22, 0x30, 0x07}};
static const GUID NV_ENC_PRESET_P4_GUID =
    {0x60bc4cb8, 0xfe66, 0x4627, {0x82, 0xea, 0x9a, 0xde, 0x65, 0xcf, 0x70, 0x06}};

typedef struct _NV_ENC_OPEN_ENCODE_SESSIONEX_PARAMS {
    uint32_t version;
    NV_ENC_DEVICE_TYPE deviceType;
    void* device;
    void* reserved;
    uint32_t apiVersion;
} NV_ENC_OPEN_ENCODE_SESSIONEX_PARAMS;

typedef struct _NV_ENC_INITIALIZE_PARAMS {
    uint32_t version;
    GUID encodeGUID;
    GUID presetGUID;
    uint32_t encodeWidth;
    uint32_t encodeHeight;
    uint32_t darWidth;
    uint32_t darHeight;
    uint32_t frameRateNum;
    uint32_t frameRateDen;
    uint32_t enableEncodeAsync;
    uint32_t enablePTD;
    void* encodeConfig;
    uint32_t maxEncodeWidth;
    uint32_t maxEncodeHeight;
    GUID tuningInfo; /* NVENC 12+ uses int tuningInfo field in newer SDK; we use preset path */
    uint32_t reserved1;
    void* reserved2[64];
} NV_ENC_INITIALIZE_PARAMS;

typedef struct _NV_ENC_CONFIG_H264 {
    uint32_t version;
    uint32_t enableTemporalSVC;
    uint32_t enableStereoMVC;
    uint32_t hierarchicalPFrames;
    uint32_t hierarchicalBFrames;
    uint32_t outputBufferingPeriodSEI;
    uint32_t outputPictureTimingSEI;
    uint32_t outputAUD;
    uint32_t disableSPSPPS;
    uint32_t outputFramePackingSEI;
    uint32_t outputRecoveryPointSEI;
    uint32_t enableIntraRefresh;
    uint32_t enableConstrainedEncoding;
    uint32_t repeatSPSPPS;
    uint32_t enableVFR;
    uint32_t enableLTR;
    uint32_t qpPrimeYZeroTransformBypassFlag;
    uint32_t useConstrainedIntraPred;
    uint32_t level;
    uint32_t idrPeriod;
    uint32_t separateColourPlaneFlag;
    uint32_t disableDeblockingFilterIDC;
    uint32_t numTemporalLayers;
    uint32_t spsId;
    uint32_t ppsId;
    uint32_t adaptiveTransformMode;
    uint32_t fmoMode;
    uint32_t bdirectMode;
    uint32_t entropyCodingMode;
    uint32_t maxNumRefFrames;
    uint32_t chromaFormatIDC;
} NV_ENC_CONFIG_H264;

typedef struct _NV_ENC_CODEC_CONFIG {
    uint32_t version;
    NV_ENC_CONFIG_H264 h264Config;
} NV_ENC_CODEC_CONFIG;

typedef struct _NV_ENC_RC_PARAMS {
    uint32_t version;
    NV_ENC_PARAMS_RC_MODE rateControlMode;
    uint32_t averageBitRate;
    uint32_t maxBitRate;
    uint32_t vbvBufferSize;
    uint32_t vbvInitialDelay;
    uint32_t enableMinQP;
    uint32_t enableMaxQP;
    uint32_t minQP[3];
    uint32_t maxQP[3];
    uint32_t enableInitialRCQP;
    uint32_t initialRCQP[3];
    uint32_t enableAQ;
    uint32_t enableLookahead;
    uint32_t disableIadapt;
    uint32_t disableBadapt;
    uint32_t enableTemporalAQ;
    uint32_t zeroReorderDelay;
    uint32_t enableNonRefP;
    uint32_t strictGOPTarget;
    uint32_t aqStrength;
    uint32_t enableExtRc;
    uint32_t enableExtLookahead;
} NV_ENC_RC_PARAMS;

typedef struct _NV_ENC_CONFIG {
    uint32_t version;
    GUID profileGUID;
    uint32_t gopLength;
    uint32_t frameIntervalP;
    uint32_t monoChromeEncoding;
    NV_ENC_PARAMS_RC_MODE rateControlMode; /* legacy */
    NV_ENC_RC_PARAMS rcParams;
    NV_ENC_CODEC_CONFIG encodeCodecConfig;
} NV_ENC_CONFIG;

typedef enum {
    NV_ENC_CAPS_EXAMPLE = 0
} NV_ENC_CAPS;

typedef struct _NV_ENC_REGISTER_RESOURCE {
    uint32_t version;
    NV_ENC_INPUT_PTR resourceToRegister;
    uint32_t resourceType; /* NV_ENC_INPUT_RESOURCE_TYPE_DIRECTX = 0x0 */
    uint32_t width;
    uint32_t height;
    uint32_t pitch;
    NV_ENC_BUFFER_FORMAT bufferFormat;
    uint32_t bufferUsage;
    NV_ENC_INPUT_PTR registeredResource;
    void* pInputResource;
    uint32_t chromaOffset[2];
    uint32_t reserved[61];
} NV_ENC_REGISTER_RESOURCE;

typedef struct _NV_ENC_MAP_INPUT_RESOURCE {
    uint32_t version;
    NV_ENC_INPUT_PTR registeredResource;
    NV_ENC_INPUT_PTR mappedResource;
    uint32_t mappedBufferFmt;
    void* mappedResourcePtr;
    uint32_t reserved[251];
} NV_ENC_MAP_INPUT_RESOURCE;

typedef struct _NV_ENC_PIC_PARAMS {
    uint32_t version;
    uint32_t inputWidth;
    uint32_t inputHeight;
    uint32_t inputPitch;
    NV_ENC_BUFFER_FORMAT bufferFmt;
    NV_ENC_INPUT_PTR inputBuffer;
    NV_ENC_OUTPUT_PTR outputBitstream;
    void* completionEvent;
    uint64_t inputTimeStamp;
    uint64_t inputDuration;
    NV_ENC_PIC_TYPE pictureType;
    uint32_t encodePicFlags;
    uint32_t frameIdx;
    uint32_t enableExternalMEHints;
    NVENC_EXTERNAL_ME_HINT* meExternalHints;
    uint32_t meHintCounts[3];
    void* alphaBuffer;
    void* qpDeltaMap;
    uint32_t qpDeltaMapSize;
    uint32_t reserved[29];
} NV_ENC_PIC_PARAMS;

typedef struct _NV_ENC_CREATE_BITSTREAM_BUFFER {
    uint32_t version;
    uint32_t size;
    NV_ENC_OUTPUT_PTR bitstreamBuffer;
    void* bitstreamBufferPtr;
    uint32_t reserved[62];
} NV_ENC_CREATE_BITSTREAM_BUFFER;

typedef struct _NV_ENC_LOCK_BITSTREAM {
    uint32_t version;
    NV_ENC_OUTPUT_PTR outputBitstream;
    uint32_t doNotWait;
    uint32_t ltrFrame;
    uint32_t getRCStats;
    uint32_t reserved;
    void* reserved2[2];
    uint32_t bitstreamSizeInBytes;
    uint64_t outputTimeStamp;
    uint64_t outputDuration;
    void* bitstreamBufferPtr;
    NV_ENC_PIC_TYPE pictureType;
    uint32_t frameAvgQP;
    uint32_t frameSatd;
    uint32_t ltrFrameIdx;
    uint32_t reserved1[259];
    void* sliceOffsets;
    uint32_t sliceOffsetsSize;
    uint32_t reserved3;
} NV_ENC_LOCK_BITSTREAM;

typedef struct _NV_ENC_SEQUENCE_PARAM_PAYLOAD {
    uint32_t version;
    uint32_t inBufferSize;
    uint32_t spsId;
    void* spsppsBuffer;
    uint32_t outSPSPPSPayloadSize;
    uint32_t reserved[250];
} NV_ENC_SEQUENCE_PARAM_PAYLOAD;

#define NVENCAPI_VERSION ((12 << 4) | 0)

typedef struct _NV_ENCODE_API_FUNCTION_LIST {
    uint32_t version;
    uint32_t reserved;
    NVENCSTATUS (NVENCAPI *nvEncOpenEncodeSession)(void*, GUID, void**);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodeGUIDCount)(void*, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodeGUIDs)(void*, GUID*, uint32_t, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodeProfileGuidCount)(void*, GUID, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodeProfileGUIDs)(void*, GUID, GUID*, uint32_t, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetInputFormatCount)(void*, GUID, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetInputFormats)(void*, GUID, NV_ENC_BUFFER_FORMAT*, uint32_t, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodeCaps)(void*, GUID, NV_ENC_CAPS, void*, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodePresetCount)(void*, GUID, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodePresetGUIDs)(void*, GUID, GUID*, uint32_t, uint32_t*);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodePresetConfig)(void*, GUID, GUID, void*);
    NVENCSTATUS (NVENCAPI *nvEncInitializeEncoder)(void*, NV_ENC_INITIALIZE_PARAMS*);
    NVENCSTATUS (NVENCAPI *nvEncCreateInputBuffer)(void*, void*);
    NVENCSTATUS (NVENCAPI *nvEncDestroyInputBuffer)(void*, NV_ENC_INPUT_PTR);
    NVENCSTATUS (NVENCAPI *nvEncCreateBitstreamBuffer)(void*, NV_ENC_CREATE_BITSTREAM_BUFFER*);
    NVENCSTATUS (NVENCAPI *nvEncDestroyBitstreamBuffer)(void*, NV_ENC_OUTPUT_PTR);
    NVENCSTATUS (NVENCAPI *nvEncEncodePicture)(void*, NV_ENC_PIC_PARAMS*);
    NVENCSTATUS (NVENCAPI *nvEncLockBitstream)(void*, NV_ENC_LOCK_BITSTREAM*);
    NVENCSTATUS (NVENCAPI *nvEncUnlockBitstream)(void*, NV_ENC_OUTPUT_PTR);
    NVENCSTATUS (NVENCAPI *nvEncLockInputBuffer)(void*, NV_ENC_INPUT_PTR, void**);
    NVENCSTATUS (NVENCAPI *nvEncUnlockInputBuffer)(void*, NV_ENC_INPUT_PTR);
    NVENCSTATUS (NVENCAPI *nvEncGetEncodeStats)(void*, void*);
    NVENCSTATUS (NVENCAPI *nvEncGetSequenceParams)(void*, NV_ENC_SEQUENCE_PARAM_PAYLOAD*);
    NVENCSTATUS (NVENCAPI *nvEncRegisterAsyncEvent)(void*, void*);
    NVENCSTATUS (NVENCAPI *nvEncUnregisterAsyncEvent)(void*, void*);
    NVENCSTATUS (NVENCAPI *nvEncMapInputResource)(void*, NV_ENC_MAP_INPUT_RESOURCE*);
    NVENCSTATUS (NVENCAPI *nvEncUnmapInputResource)(void*, NV_ENC_INPUT_PTR);
    NVENCSTATUS (NVENCAPI *nvEncDestroyEncoder)(void*);
    NVENCSTATUS (NVENCAPI *nvEncInvalidateRefFrames)(void*, uint64_t);
    NVENCSTATUS (NVENCAPI *nvEncOpenEncodeSessionEx)(NV_ENC_OPEN_ENCODE_SESSIONEX_PARAMS*, void**);
    NVENCSTATUS (NVENCAPI *nvEncRegisterResource)(void*, NV_ENC_REGISTER_RESOURCE*);
    NVENCSTATUS (NVENCAPI *nvEncUnregisterResource)(void*, NV_ENC_INPUT_PTR);
    NVENCSTATUS (NVENCAPI *nvEncReconfigureEncoder)(void*, void*);
} NV_ENCODE_API_FUNCTION_LIST;

typedef NVENCSTATUS (NVENCAPI *NvEncodeAPICreateInstance_t)(NV_ENCODE_API_FUNCTION_LIST*);

#define NV_ENC_OPEN_ENCODE_SESSION_EX_PARAMS_VER 0x00000001
#define NV_ENC_INITIALIZE_PARAMS_VER 0x00000006
#define NV_ENC_CONFIG_VER 0x00000007
#define NV_ENC_CODEC_CONFIG_VER 0x00000004
#define NV_ENC_CONFIG_H264_VER 0x00000004
#define NV_ENC_RC_PARAMS_VER 0x00000002
#define NV_ENC_REGISTER_RESOURCE_VER 0x00000004
#define NV_ENC_MAP_INPUT_RESOURCE_VER 0x00000004
#define NV_ENC_PIC_PARAMS_VER 0x00000006
#define NV_ENC_CREATE_BITSTREAM_BUFFER_VER 0x00000001
#define NV_ENC_LOCK_BITSTREAM_VER 0x00000001
#define NV_ENCODE_API_FUNCTION_LIST_VER 0x00000008

#define NV_ENC_PIC_FLAG_FORCEIDR 0x00000004
#define NV_ENC_INPUT_RESOURCE_TYPE_DIRECTX 0x0

#ifdef __cplusplus
}
#endif

#endif
