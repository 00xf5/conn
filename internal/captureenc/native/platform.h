#ifndef CONNECT_PLATFORM_H
#define CONNECT_PLATFORM_H

#ifndef WINVER
#define WINVER 0x0A00
#endif
#ifndef _WIN32_WINNT
#define _WIN32_WINNT 0x0A00
#endif

#define WIN32_LEAN_AND_MEAN
#define COBJMACROS
#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>
#include <stdint.h>

static inline uint64_t platform_qpc_freq(void) {
    LARGE_INTEGER f;
    QueryPerformanceFrequency(&f);
    return (uint64_t)f.QuadPart;
}

static inline uint64_t platform_qpc_now(void) {
    LARGE_INTEGER t;
    QueryPerformanceCounter(&t);
    return (uint64_t)t.QuadPart;
}

static inline uint64_t platform_qpc_to_us(uint64_t qpc, uint64_t freq) {
    if (freq == 0) return 0;
    return (qpc * 1000000ULL) / freq;
}

#endif
