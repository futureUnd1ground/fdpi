#pragma once
#include <stddef.h>
#define CCP_POSIX_TO_WIN_W 0x100000
#define CCP_WIN_W_TO_POSIX 0x80000
#define CCP_WIN_A_TO_POSIX 0x40000
#define CCP_ABSOLUTE 0x1000
#ifdef __cplusplus
extern "C" {
#endif
size_t cygwin_conv_path(unsigned what, const void *from, void *to, size_t size);
#ifdef __cplusplus
}
#endif
