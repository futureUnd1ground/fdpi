#pragma once
#include <stdint.h>
struct tcphdr { uint16_t th_sport, th_dport; uint32_t th_seq, th_ack; uint8_t th_x2:4, th_off:4; uint8_t th_flags; uint16_t th_win, th_sum, th_urp; };
#define TH_FIN 0x01
#define TH_SYN 0x02
#define TH_RST 0x04
#define TH_PUSH 0x08
#define TH_ACK 0x10
#define TH_URG 0x20
