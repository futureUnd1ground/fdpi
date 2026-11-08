#pragma once
#define LOG_PID 1
#define LOG_DAEMON 24
#define LOG_ERR 3
#define LOG_WARNING 4
#define LOG_INFO 6
#define LOG_DEBUG 7
static void openlog(const char *ident, int option, int facility) { (void)ident; (void)option; (void)facility; }
static void syslog(int priority, const char *fmt, ...) { (void)priority; (void)fmt; }
static int setlogmask(int mask) { (void)mask; return 0; }
