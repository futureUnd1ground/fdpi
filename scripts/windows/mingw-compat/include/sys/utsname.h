#pragma once
struct utsname { char sysname[64]; char nodename[64]; char release[64]; char version[128]; char machine[64]; };
#ifdef __cplusplus
extern "C" {
#endif
int uname(struct utsname *buf);
#ifdef __cplusplus
}
#endif
