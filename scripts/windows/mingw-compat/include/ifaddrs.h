#pragma once
struct ifaddrs { struct ifaddrs *ifa_next; char *ifa_name; unsigned int ifa_flags; struct sockaddr *ifa_addr; struct sockaddr *ifa_netmask; struct ifaddrs *__ifa_pad1; struct sockaddr *ifa_broadaddr; struct sockaddr *ifa_dstaddr; void *__ifa_pad2; };
#ifdef __cplusplus
extern "C" {
#endif
int getifaddrs(struct ifaddrs **ifap);
void freeifaddrs(struct ifaddrs *ifa);
#ifdef __cplusplus
}
#endif
