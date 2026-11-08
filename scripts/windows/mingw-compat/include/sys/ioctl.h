#pragma once
#ifndef FIONREAD
#define FIONREAD 0x4004667f
#endif
static int ioctl(int fd, unsigned long req, ...) { (void)fd; (void)req; return -1; }
