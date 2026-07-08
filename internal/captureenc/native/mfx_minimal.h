#ifndef CONNECT_MFX_MINIMAL_H
#define CONNECT_MFX_MINIMAL_H

/* Official Intel Media SDK struct layouts — required for MFXVideoENCODE_Query/Init.
 * Hand-rolled structs caused access violations inside libmfx on 64-bit Windows. */

#ifndef MFX_VERSION
#define MFX_VERSION 1035
#endif

#include "mfxdefs.h"
#include "mfxcommon.h"
#include "mfxsession.h"
#include "mfxstructures.h"

/* Dynamic loader function pointers (libmfx is loaded at runtime). */
typedef mfxStatus (*MFXInit_t)(mfxIMPL impl, mfxVersion* ver, mfxSession* session);
typedef mfxStatus (*MFXClose_t)(mfxSession session);
typedef mfxStatus (*MFXVideoCORE_SetHandle_t)(mfxSession session, mfxHandleType type, mfxHDL hdl);
typedef mfxStatus (*MFXVideoENCODE_Query_t)(mfxSession session, mfxVideoParam* in, mfxVideoParam* out);
typedef mfxStatus (*MFXVideoENCODE_Init_t)(mfxSession session, mfxVideoParam* par);
typedef mfxStatus (*MFXVideoENCODE_Close_t)(mfxSession session);
typedef mfxStatus (*MFXVideoENCODE_Reset_t)(mfxSession session, mfxVideoParam* par);
typedef mfxStatus (*MFXVideoENCODE_EncodeFrameAsync_t)(mfxSession session, mfxEncodeCtrl* ctrl,
    mfxFrameSurface1* surface, mfxBitstream* bs, mfxSyncPoint* syncp);
typedef mfxStatus (*MFXVideoCORE_SyncOperation_t)(mfxSession session, mfxSyncPoint syncp, mfxU32 wait);

static inline mfxU16 mfx_align16(mfxU16 v) { return (mfxU16)((v + 15) & ~15); }

#endif
