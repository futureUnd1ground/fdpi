#pragma once
#include <stdint.h>
#include <netinet/in.h>
struct ip6_hdr {
    union {
        struct {
            uint32_t ip6_un1_flow;
            uint16_t ip6_un1_plen;
            uint8_t  ip6_un1_nxt;
            uint8_t  ip6_un1_hlim;
        } ip6_un1;
        uint8_t ip6_un2_vfc;
    } ip6_ctlun;
    struct in6_addr ip6_src;
    struct in6_addr ip6_dst;
};
#define ip6_vfc   ip6_ctlun.ip6_un2_vfc
#define ip6_flow  ip6_ctlun.ip6_un1.ip6_un1_flow
#define ip6_plen  ip6_ctlun.ip6_un1.ip6_un1_plen
#define ip6_nxt   ip6_ctlun.ip6_un1.ip6_un1_nxt
#define ip6_hlim  ip6_ctlun.ip6_un1.ip6_un1_hlim
#define ip6_hops  ip6_ctlun.ip6_un1.ip6_un1_hlim
#define IP6_VERSION 6
struct ip6_frag {
    uint8_t  ip6f_nxt;
    uint8_t  ip6f_reserved;
    uint16_t ip6f_offlg;
    uint32_t ip6f_ident;
};
#define IP6F_OFF_MASK 0xfff8
#define IP6F_MORE_FRAG 0x0001
