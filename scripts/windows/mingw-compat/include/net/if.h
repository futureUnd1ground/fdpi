#pragma once
#ifndef IFNAMSIZ
#define IFNAMSIZ 256
#endif
#ifndef _WINSOCK2API_
#define WIN32_LEAN_AND_MEAN 1
#include <winsock2.h>
#include <ws2tcpip.h>
#endif
#include <netioapi.h>
