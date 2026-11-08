#pragma once
struct group { char *gr_name; char *gr_passwd; int gr_gid; char **gr_mem; };
static struct group *getgrnam(const char *name) { (void)name; return (void*)0; }
static struct group *getgrgid(int gid) { (void)gid; return (void*)0; }
