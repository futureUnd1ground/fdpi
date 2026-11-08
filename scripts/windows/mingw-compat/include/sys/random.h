#pragma once
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
int getrandom(void *buf, size_t buflen, unsigned int flags);
#ifdef __cplusplus
}
#endif
