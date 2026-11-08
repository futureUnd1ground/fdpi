#pragma once
struct passwd { char *pw_name; char *pw_passwd; int pw_uid; int pw_gid; char *pw_gecos; char *pw_dir; char *pw_shell; };
static struct passwd *getpwnam(const char *name) { (void)name; return (void*)0; }
static struct passwd *getpwuid(int uid) { (void)uid; return (void*)0; }
