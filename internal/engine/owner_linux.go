package engine

import (
	"os"
	"syscall"
)

func rootOwned(s os.FileInfo) bool { v, ok := s.Sys().(*syscall.Stat_t); return ok && v.Uid == 0 }
