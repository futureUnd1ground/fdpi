#pragma once
#include <stddef.h>
#define WRDE_NOCMD 1
#define WRDE_BADCHAR 1
#define WRDE_BADVAL 2
#define WRDE_CMDSUB 3
#define WRDE_NOSPACE 4
#define WRDE_SYNTAX 5
typedef struct { size_t we_wordc; char **we_wordv; size_t we_offs; } wordexp_t;
#ifdef __cplusplus
extern "C" {
#endif
int wordexp(const char *s, wordexp_t *p, int flags);
void wordfree(wordexp_t *p);
#ifdef __cplusplus
}
#endif
