package service

import (
	"context"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"os/exec"
	"strings"
)

// RequireCurrentBinary prevents mixing a new controller/config with an old daemon.
func RequireCurrentBinary(ctx context.Context) error {
	b, e := exec.CommandContext(ctx, "/usr/bin/fdpi", "--version").Output()
	if e != nil {
		return fmt.Errorf("системная часть FDPI не установлена; откройте установщик клавишей i: %w", e)
	}
	fields := strings.Fields(string(b))
	if len(fields) != 2 || fields[0] != "FDPI" || fields[1] != config.ApplicationVersion {
		return fmt.Errorf("системная часть %s; интерфейс %s. Сначала обновите установку: i или fdpi setup", strings.TrimSpace(string(b)), config.ApplicationVersion)
	}
	return nil
}
