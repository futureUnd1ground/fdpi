package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/network"
)

var installedFiles = []string{
	"/usr/bin/fdpi", "/usr/lib/fdpi/nfqws", "/usr/lib/fdpi/nfqws2",
	"/usr/lib/fdpi/lua/zapret-lib.lua", "/usr/lib/fdpi/lua/zapret-antidpi.lua",
	"/usr/lib/systemd/system/fdpi.service",
	"/usr/share/licenses/fdpi/LICENSE", "/usr/share/licenses/fdpi/ZAPRET-LICENSE",
	"/usr/share/doc/fdpi/README.md", "/usr/share/doc/fdpi/INSTALL_RU.md",
}

// Uninstall removes only the installation manifest, retaining all configurations.
func Uninstall(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("для удаления нужны права администратора")
	}
	if _, err := os.Stat("/usr/bin/pacman"); err == nil {
		out, err := exec.CommandContext(ctx, "/usr/bin/pacman", "-Qqo", "/usr/bin/fdpi").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf("FDPI установлен пакетом %s; удалите его через sudo pacman -R %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out)))
		}
	}
	lock, err := accountingLock()
	if err != nil {
		return err
	}
	defer Unlock(lock)
	for _, path := range installedFiles {
		parent := filepath.Dir(path)
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := engine.Trusted(parent); err != nil {
			return err
		}
	}
	return uninstall(ctx, "", Command, network.Cleanup)
}

func uninstall(ctx context.Context, root string, command func(context.Context, ...string) (string, error), cleanup func(context.Context) error) error {
	unit := filepath.Join(root, "usr/lib/systemd/system/fdpi.service")
	if _, err := os.Lstat(unit); err == nil {
		if _, err = command(ctx, "disable", "--now", "fdpi.service"); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := cleanup(ctx); err != nil {
		return fmt.Errorf("очистка правил FDPI: %w", err)
	}
	for _, file := range installedFiles {
		if err := os.Remove(filepath.Join(root, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("удаление %s: %w", file, err)
		}
	}
	for _, dir := range []string{"usr/lib/fdpi/lua", "usr/lib/fdpi", "usr/share/licenses/fdpi", "usr/share/doc/fdpi"} {
		// Nonempty directories can contain unrelated files: preserve them.
		_ = os.Remove(filepath.Join(root, dir))
	}
	_, err := command(ctx, "daemon-reload")
	return err
}
