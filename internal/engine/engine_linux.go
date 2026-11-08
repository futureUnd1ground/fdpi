package engine

import (
	"context"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const Path = "/usr/lib/fdpi/nfqws2"

// platformArgs возвращает системный префикс argv движка nfqws2 (NFQUEUE).
func platformArgs(tcp, udp bool) []string {
	return []string{fmt.Sprintf("--qnum=%d", network.Queue), "--uid=0:0", "--fwmark=" + network.Mark, "--bind-fix4", "--bind-fix6"}
}

func Check(ctx context.Context, path string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	h, e := exec.CommandContext(ctx, path, "--help").CombinedOutput()
	if e != nil && len(h) == 0 {
		return fmt.Errorf("движок: %w", e)
	}
	for _, feature := range []string{"--dry-run", "--lua-desync=", "--lua-init=", "--bind-fix4", "--bind-fix6", "--hostlist=", "--new", "--pidfile=", "--uid=", "--fwmark="} {
		if !strings.Contains(string(h), feature) {
			return fmt.Errorf("несовместимый nfqws: нет параметра %s; требуется nfqws2", feature)
		}
	}
	a := []string{"--dry-run"}
	for _, arg := range args {
		if os.Geteuid() != 0 && arg == "--uid=0:0" {
			continue
		}
		a = append(a, arg)
	}
	b, e := exec.CommandContext(ctx, path, a...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("проверка параметров nfqws: %w: %s", e, b)
	}
	return nil
}
func Trusted(path string) error {
	// Resolve each component: root must own it and group/other must not be able to replace it.
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("в привилегированном пути запрещена символическая ссылка: %s", p)
		}
		if st.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("привилегированный путь доступен для записи группе или другим пользователям: %s", p)
		}
		if !rootOwned(st) {
			return fmt.Errorf("владельцем привилегированного пути должен быть root: %s", p)
		}
		if p == "/" {
			break
		}
	}
	return nil
}
