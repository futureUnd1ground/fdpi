// mingw compatibility shims for winws2 (zapret2 nfq2 built with mingw instead of cygwin)
#define _WIN32_WINNT 0x0600
#include <windows.h>
#include <winsock2.h>
#include <ws2tcpip.h>
#include <iphlpapi.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include "include/sys/utsname.h"
#include "include/ifaddrs.h"
#include "include/wordexp.h"
#include "include/sys/cygwin.h"
#include "include/libgen.h"
#include <stdio.h>

// ---- uname ----
int uname(struct utsname *buf) {
    OSVERSIONINFOEXA vi; DWORD sz = sizeof(buf->nodename);
    memset(&vi, 0, sizeof(vi)); vi.dwOSVersionInfoSize = sizeof(vi);
    GetVersionExA((OSVERSIONINFOA*)&vi);
    strcpy(buf->sysname, "Windows");
    GetComputerNameA(buf->nodename, &sz);
    snprintf(buf->release, sizeof(buf->release), "%lu.%lu.%lu", vi.dwMajorVersion, vi.dwMinorVersion, vi.dwBuildNumber);
    strcpy(buf->version, "Windows NT");
#if defined(_M_X64) || defined(__x86_64__)
    strcpy(buf->machine, "x86_64");
#else
    strcpy(buf->machine, "i686");
#endif
    return 0;
}

// ---- getifaddrs over GetAdaptersAddresses ----
int getifaddrs(struct ifaddrs **ifap) {
    ULONG flags = GAA_FLAG_INCLUDE_PREFIX | GAA_FLAG_SKIP_ANYCAST | GAA_FLAG_SKIP_MULTICAST | GAA_FLAG_SKIP_DNS_SERVER;
    ULONG buflen = 16384;
    IP_ADAPTER_ADDRESSES *aa = malloc(buflen);
    struct ifaddrs *head = NULL, *tail = NULL;
    *ifap = NULL;
    if (!aa) return -1;
    if (GetAdaptersAddresses(AF_UNSPEC, flags, NULL, aa, &buflen) != ERROR_SUCCESS) { free(aa); return -1; }
    for (IP_ADAPTER_ADDRESSES *a = aa; a; a = a->Next) {
        for (IP_ADAPTER_UNICAST_ADDRESS *u = a->FirstUnicastAddress; u; u = u->Next) {
            struct ifaddrs *ifa = calloc(1, sizeof(*ifa));
            if (!ifa) break;
            ifa->ifa_name = _strdup(a->AdapterName);
            ifa->ifa_flags = IFF_UP;
            if (!(a->OperStatus & IfOperStatusUp)) ifa->ifa_flags &= ~IFF_UP;
            if (a->IfType == IF_TYPE_SOFTWARE_LOOPBACK) ifa->ifa_flags |= IFF_LOOPBACK;
            size_t salen = u->Address.lpSockaddr->sa_family == AF_INET ? sizeof(struct sockaddr_in) : sizeof(struct sockaddr_in6);
            ifa->ifa_addr = malloc(salen); memcpy(ifa->ifa_addr, u->Address.lpSockaddr, salen);
            if (u->Address.lpSockaddr->sa_family == AF_INET) {
                struct sockaddr_in *nm = calloc(1, sizeof(struct sockaddr_in));
                nm->sin_family = AF_INET;
                ULONG plen = u->OnLinkPrefixLength;
                ULONG mask = plen ? (0xFFFFFFFFu << (32 - plen)) : 0;
                nm->sin_addr.s_addr = htonl(mask);
                ifa->ifa_netmask = (struct sockaddr*)nm;
            } else {
                struct sockaddr_in6 *nm = calloc(1, sizeof(struct sockaddr_in6));
                nm->sin6_family = AF_INET6;
                ULONG plen = u->OnLinkPrefixLength;
                for (ULONG i = 0; i < 16; i++) {
                    ULONG bits = plen > i * 8 ? (plen - i * 8) : 0;
                    nm->sin6_addr.s6_addr[i] = bits >= 8 ? 0xFF : (bits ? (0xFF << (8 - bits)) : 0);
                }
                ifa->ifa_netmask = (struct sockaddr*)nm;
            }
            ifa->ifa_broadaddr = NULL; ifa->ifa_dstaddr = NULL;
            if (!head) head = tail = ifa; else { tail->ifa_next = ifa; tail = ifa; }
        }
    }
    free(aa);
    *ifap = head;
    return 0;
}
void freeifaddrs(struct ifaddrs *ifa) {
    while (ifa) {
        struct ifaddrs *n = ifa->ifa_next;
        free(ifa->ifa_name); free(ifa->ifa_addr); free(ifa->ifa_netmask); free(ifa);
        ifa = n;
    }
}

// ---- wordexp: simple whitespace tokenizer (no expansion; FDPI does not use @config files) ----
int wordexp(const char *s, wordexp_t *p, int flags) {
    (void)flags;
    char *copy = _strdup(s ? s : "");
    if (!copy) return WRDE_NOSPACE;
    size_t cap = 16, n = 0;
    char **v = malloc(cap * sizeof(char*));
    if (!v) { free(copy); return WRDE_NOSPACE; }
    char *save = NULL;
    for (char *t = strtok_r(copy, " \t\r\n", &save); t; t = strtok_r(NULL, " \t\r\n", &save)) {
        if (n == cap) { cap *= 2; char **nv = realloc(v, cap * sizeof(char*)); if (!nv) { free(v); free(copy); return WRDE_NOSPACE; } v = nv; }
        v[n++] = _strdup(t);
    }
    p->we_wordc = n; p->we_wordv = v; p->we_offs = 0;
    free(copy);
    return 0;
}
void wordfree(wordexp_t *p) {
    for (size_t i = 0; i < p->we_wordc; i++) free(p->we_wordv[i]);
    free(p->we_wordv); p->we_wordc = 0; p->we_wordv = NULL;
}

// ---- cygwin_conv_path ----
/* Contract (matches cygwin): with to==NULL returns required buffer size;
   with to!=NULL writes the converted path and returns 0 on success, -1 on error. */
size_t cygwin_conv_path(unsigned what, const void *from, void *to, size_t size) {
    if (!from) return (size_t)-1;
    if (what & CCP_POSIX_TO_WIN_W) {
        int l = MultiByteToWideChar(CP_UTF8, 0, (const char*)from, -1, NULL, 0);
        if (!to) return (size_t)l * 2;
        if (size < (size_t)l * 2) return (size_t)-1;
        MultiByteToWideChar(CP_UTF8, 0, (const char*)from, -1, (wchar_t*)to, l);
        return 0;
    }
    {
        const char *s = (const char*)from;
        static char buf[4096];
        if (what & CCP_WIN_W_TO_POSIX) {
            WideCharToMultiByte(CP_UTF8, 0, (const wchar_t*)from, -1, buf, sizeof(buf), NULL, NULL);
            s = buf;
        }
        size_t l = strlen(s) + 1;
        if (!to) return l;
        if (size < l) return (size_t)-1;
        memcpy(to, s, l);
        return 0;
    }
}

// ---- libgen ----
static char *g_base, *g_dir;
char *basename(char *path) {
    char *p = strrchr(path, '\\'); char *q = strrchr(path, '/');
    char *sep = p > q ? p : q;
    free(g_base);
    g_base = _strdup(sep ? sep + 1 : path);
    return g_base;
}
char *dirname(char *path) {
    char tmp[4096]; strncpy(tmp, path, sizeof(tmp) - 1); tmp[sizeof(tmp)-1] = 0;
    char *p = strrchr(tmp, '\\'); char *q = strrchr(tmp, '/');
    char *sep = p > q ? p : q;
    if (sep) *sep = 0; else strcpy(tmp, ".");
    free(g_dir); g_dir = _strdup(tmp);
    return g_dir;
}

// ---- getrandom ----
int getrandom(void *buf, size_t buflen, unsigned int flags) {
    (void)flags;
    unsigned char *p = buf;
    while (buflen) {
        unsigned int r = rand();
        size_t n = buflen < sizeof(r) ? buflen : sizeof(r);
        memcpy(p, &r, n);
        p += n; buflen -= n;
    }
    return 0;
}

// ---- clock_gettime/nanosleep for older mingw-w64 (Ubuntu) ----
#ifndef CLOCK_REALTIME
#define ZC_HAVE_CLOCK_SHIM 1
#endif
#ifdef ZC_HAVE_CLOCK_SHIM
int clock_gettime(clockid_t clk, struct timespec *ts) {
    (void)clk;
    FILETIME ft;
    GetSystemTimePreciseAsFileTime(&ft);
    unsigned long long ticks = ((unsigned long long)ft.dwHighDateTime << 32) | ft.dwLowDateTime;
    unsigned long long secs = ticks / 10000000ULL - 11644473600ULL;
    ts->tv_sec = (time_t)secs;
    ts->tv_nsec = (long)((ticks % 10000000ULL) * 100);
    return 0;
}
int nanosleep(const struct timespec *req, struct timespec *rem) {
    (void)rem;
    Sleep((DWORD)(req->tv_sec * 1000 + req->tv_nsec / 1000000));
    return 0;
}
int usleep(unsigned int usec) { Sleep(usec / 1000 + 1); return 0; }
unsigned int sleep(unsigned int seconds) { Sleep(seconds * 1000); return 0; }
#endif
