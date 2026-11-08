#pragma once
#include <stdint.h>
struct udphdr { uint16_t uh_sport, uh_dport, uh_ulen, uh_sum; };
