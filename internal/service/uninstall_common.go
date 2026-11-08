package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// RemoveUserLauncher removes only the user's Linux FDPI symlink, never its target.
func RemoveUserLauncher() error {
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".local/bin/fdpi")
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return os.Remove(path)
}
