#pragma once
#include <stdint.h>
#include <netinet/in.h>
struct ip {
#if defined(__BYTE_ORDER__) && __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    uint8_t ip_hl:4, ip_v:4;
#else
    uint8_t ip_v:4, ip_hl:4;
#endif
    uint8_t ip_tos;
    uint16_t ip_len, ip_id, ip_off;
    uint8_t ip_ttl, ip_p;
    uint16_t ip_sum;
    struct in_addr ip_src, ip_dst;
};
#define IP_RF 0x8000
#define IP_DF 0x4000
#define IP_MF 0x2000
#define IP_OFFMASK 0x1fff
#define IP_HDRLEN(ip) ((ip)->ip_hl * 4)
#ifndef IPVERSION
#define IPVERSION 4
#endif
