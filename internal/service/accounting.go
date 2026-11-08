package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const accountingFile = "/etc/sysctl.d/70-fdpi.conf"
const accountingKey = "net.netfilter.nf_conntrack_acct"

func accountingLock() (*os.File, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("нужны права root для настройки conntrack")
	}
	if e := engine.Trusted("/etc/fdpi"); e != nil {
		return nil, e
	}
	return Lock("/etc/fdpi/.operation.lock")
}

// EnableAccounting changes only the one explicitly requested conntrack sysctl.
func EnableAccounting(ctx context.Context) error {
	lock, e := accountingLock()
	if e != nil {
		return e
	}
	defer Unlock(lock)
	cmd := exec.CommandContext(ctx, "/usr/bin/sysctl", "-w", accountingKey+"=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PersistAccounting creates a fixed sysctl file without replacing existing settings.
func PersistAccounting() error {
	lock, e := accountingLock()
	if e != nil {
		return e
	}
	defer Unlock(lock)
	if e = engine.Trusted("/etc/sysctl.d"); e != nil {
		return e
	}
	if _, e = os.Lstat(accountingFile); e == nil {
		if e = engine.Trusted(accountingFile); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return writeAccountingFile(accountingFile)
}
func writeAccountingFile(path string) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(e, os.ErrExist) {
		st, se := os.Lstat(path)
		if se != nil {
			return se
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("отказ менять ссылку или специальный файл: %s", path)
		}
		existing, re := os.Open(path)
		if re != nil {
			return re
		}
		defer existing.Close()
		b, re := io.ReadAll(io.LimitReader(existing, 65537))
		if re != nil {
			return re
		}
		if len(b) > 65536 {
			return fmt.Errorf("существующий файл sysctl слишком большой; проверьте его вручную")
		}
		value := ""
		for _, line := range strings.Split(string(b), "\n") {
			line, _, _ = strings.Cut(line, "#")
			key, v, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(key) == accountingKey {
				value = strings.TrimSpace(v)
			}
		}
		if value == "1" {
			return nil
		}
		return fmt.Errorf("файл %s уже существует и не включает нужный параметр; он сохранён без изменений. Исправьте его через sudoedit", path)
	}
	if e != nil {
		return e
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, e = f.WriteString("# Создано интерактивным мастером FDPI.\n" + accountingKey + "=1\n"); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	if e = dir.Sync(); e != nil {
		return e
	}
	ok = true
	return nil
}
