package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/futureUnd1ground/fdpi/internal/engine"
	"golang.org/x/sys/windows"
)

// Uninstall removes the registered service and the portable installation manifest.
// Windows retains the running executable until reboot; user settings stay intact.
func Uninstall(ctx context.Context) error {
	if !IsAdmin() {
		return fmt.Errorf("нужны права администратора")
	}
	if err := EnsureStateDir(); err != nil {
		return err
	}
	lock, err := Lock(OperationLockPath())
	if err != nil {
		return err
	}
	defer Unlock(lock)
	state := State(ctx)
	if state.Detail != "" {
		return fmt.Errorf("проверка службы: %s", state.Detail)
	}
	if state.Installed {
		if err := Control(ctx, "stop", false); err != nil {
			return err
		}
		if err := Remove(ctx); err != nil {
			return err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	names := []string{"winws2.exe", "WinDivert.dll", "WinDivert64.sys", "lua/zapret-lib.lua", "lua/zapret-antidpi.lua", "ZAPRET-LICENSE", "engine.commit", filepath.Base(exe)}
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := engine.Trusted(filepath.Dir(path)); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			p, e := windows.UTF16PtrFromString(path)
			if e != nil {
				return e
			}
			if e = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT); e != nil {
				return fmt.Errorf("удаление %s: %w", name, e)
			}
		}
	}
	return nil
}
