#pragma once
/* Force-include for building zapret2 nfq2 (winws2) with mingw-w64.
   Must be included before ANY system header. */
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN 1
#endif
#ifndef _WINSOCK2API_
#include <winsock2.h>
#include <ws2tcpip.h>
#endif

#include <stddef.h>
#include <string.h>
#include <stdlib.h>
#include <time.h>
#include <fcntl.h>
#include <signal.h>
#include <direct.h>
#include <windows.h>
#include <stdio.h>

#ifndef MAX
#define MAX(a,b) ((a)>(b)?(a):(b))
#endif
#ifndef MIN
#define MIN(a,b) ((a)<(b)?(a):(b))
#endif
#ifndef st_mtim
#define st_mtim st_mtime
#endif
#ifndef GRND_NONBLOCK
#define GRND_NONBLOCK 1
#endif
#ifndef O_NONBLOCK
#define O_NONBLOCK 0x4000
#endif
#ifndef SIGHUP
#define SIGHUP 1
#endif
#ifndef SIGUSR1
#define SIGUSR1 10
#endif
#ifndef SIGUSR2
#define SIGUSR2 12
#endif
#ifndef SIGPIPE
#define SIGPIPE 13
#endif
#ifndef IPVERSION
#define IPVERSION 4
#endif
#ifndef S_IFLNK
#define S_IFLNK 0xA000
#endif
#ifndef S_IFSOCK
#define S_IFSOCK 0xC000
#endif
#ifndef IPTOS_ECN_MASK
#define IPTOS_ECN_MASK 0x03
#endif
#ifndef IPTOS_ECN_ECT1
#define IPTOS_ECN_ECT1 0x01
#endif
#ifndef IPTOS_ECN_ECT0
#define IPTOS_ECN_ECT0 0x02
#endif
#ifndef IPTOS_ECN_CE
#define IPTOS_ECN_CE 0x03
#endif
#ifndef ESHUTDOWN
#define ESHUTDOWN 108
#endif
#ifndef LOG_USER
#define LOG_USER 8
#endif

typedef int zc_id_t;
#define uid_t zc_id_t
#define gid_t zc_id_t
typedef unsigned int sigset_t;

#define srandom(s) srand((unsigned)(s))

static const GUID ZC_FOLDERID_LocalAppDataLow = {0xA520A1A4, 0x1780, 0x4FF6, {0xBD, 0x18, 0x16, 0x73, 0x43, 0xC5, 0xAF, 0x16}};
#ifndef FOLDERID_LocalAppDataLow
#define FOLDERID_LocalAppDataLow ZC_FOLDERID_LocalAppDataLow
#endif

static long zc_random(void) { return (long)rand(); }
#define random zc_random

static void *zc_memmem(const void *h, size_t hl, const void *n, size_t nl) {
    const char *p;
    if (!nl) return (void *)h;
    if (nl > hl) return 0;
    for (p = h; hl >= nl; p++, hl--) if (!memcmp(p, n, nl)) return (void *)p;
    return 0;
}
#define memmem zc_memmem

static char *zc_strcasestr(const char *h, const char *n) {
    size_t nl = strlen(n);
    for (; *h; h++) {
        size_t i;
        for (i = 0; i < nl; i++) {
            char a = h[i], b = n[i];
            if (a >= 'A' && a <= 'Z') a += 32;
            if (b >= 'A' && b <= 'Z') b += 32;
            if (a != b) break;
        }
        if (i == nl) return (char *)h;
    }
    return 0;
}
#define strcasestr zc_strcasestr

static struct tm *zc_localtime_r(const time_t *t, struct tm *r) { return localtime_s(r, t) ? NULL : r; }
#define localtime_r zc_localtime_r
static struct tm *zc_gmtime_r(const time_t *t, struct tm *r) { return gmtime_s(r, t) ? NULL : r; }
#define gmtime_r zc_gmtime_r
static time_t zc_timegm(struct tm *t) { return _mkgmtime(t); }
#define timegm zc_timegm

static int zc_setenv(const char *n, const char *v, int ow) { (void)ow; return SetEnvironmentVariableA(n, v) ? 0 : -1; }
#define setenv zc_setenv

static char *zc_realpath(const char *p, char *out) {
    char tmp[4096];
    if (_fullpath(tmp, p, sizeof(tmp) - 1)) {
        if (!out) out = malloc(4096);
        if (out) strcpy(out, tmp);
        return out;
    }
    return NULL;
}
#define realpath zc_realpath

/* sigaction emulation over signal(); struct tag and function name live in
   separate C namespaces, so no macros are needed. */
struct sigaction {
    void (*sa_handler)(int);
    unsigned int sa_mask;
    int sa_flags;
    void (*sa_sigaction)(int, void *, void *);
};
#define sigemptyset(m) (*(m) = 0)
static int sigaction(int sig, const struct sigaction *act, struct sigaction *old) {
    (void)old;
    return signal(sig, act ? act->sa_handler : SIG_DFL) == SIG_ERR ? -1 : 0;
}

static int zc_getsockopt_wrap(SOCKET s, int l, int o, void *v, int *n) { return getsockopt(s, l, o, (char *)v, n); }
static int zc_setsockopt_wrap(SOCKET s, int l, int o, const void *v, int n) { return setsockopt(s, l, o, (const char *)v, n); }
#define getsockopt zc_getsockopt_wrap
#define setsockopt zc_setsockopt_wrap

#define mkdir(p, m) _mkdir(p)

/* clock_gettime/nanosleep/usleep for older mingw-w64 runtimes (e.g. Ubuntu) */
#ifndef CLOCK_REALTIME
#define CLOCK_REALTIME 0
#define CLOCK_MONOTONIC 1
typedef int clockid_t;
int clock_gettime(clockid_t clk, struct timespec *ts);
int nanosleep(const struct timespec *req, struct timespec *rem);
int usleep(unsigned int usec);
unsigned int sleep(unsigned int seconds);
#endif
