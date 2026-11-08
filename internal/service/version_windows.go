package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/svc/mgr"
)

// RequireCurrentBinary prevents mixing a new controller with an old installed service.
func RequireCurrentBinary(ctx context.Context) error {
	m, e := mgr.Connect()
	if e != nil {
		return fmt.Errorf("подключение к диспетчеру служб: %w", e)
	}
	defer m.Disconnect()
	s, e := m.OpenService(serviceName)
	if e != nil {
		return fmt.Errorf("системная часть FDPI не установлена; откройте установщик клавишей i или выполните fdpi setup: %w", e)
	}
	defer s.Close()
	cfg, e := s.Config()
	if e != nil {
		return e
	}
	installed := serviceBinaryPath(cfg.BinaryPathName)
	current, e := os.Executable()
	if e != nil {
		return e
	}
	canonical := func(p string) string {
		if real, e := filepath.EvalSymlinks(p); e == nil {
			p = real
		}
		return strings.ToLower(filepath.Clean(p))
	}
	if canonical(installed) != canonical(current) {
		return fmt.Errorf("служба fdpi использует другой исполняемый файл: %s; текущий: %s. Переустановите службу: fdpi setup", installed, current)
	}
	return nil
}

// serviceBinaryPath extracts the executable from a command line like "C:\...\fdpi.exe" daemon.
func serviceBinaryPath(commandLine string) string {
	commandLine = strings.TrimSpace(commandLine)
	if strings.HasPrefix(commandLine, "\"") {
		if i := strings.Index(commandLine[1:], "\""); i >= 0 {
			return commandLine[1 : 1+i]
		}
		return strings.Trim(commandLine, "\"")
	}
	if i := strings.IndexByte(commandLine, ' '); i >= 0 {
		return commandLine[:i]
	}
	return commandLine
}
