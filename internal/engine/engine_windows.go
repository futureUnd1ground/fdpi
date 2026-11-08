package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Path is winws2.exe shipped next to fdpi.exe; WinDivert filters live inside winws2 itself.
var Path = func() string {
	exe, e := os.Executable()
	if e != nil {
		return "winws2.exe"
	}
	if real, e := filepath.EvalSymlinks(exe); e == nil {
		exe = real
	}
	return filepath.Join(filepath.Dir(exe), "winws2.exe")
}()

// platformArgs возвращает системный префикс argv движка winws2 (WinDivert).
func platformArgs(tcp, udp bool) []string {
	var args []string
	if tcp {
		args = append(args, "--wf-tcp-out=80,443")
	}
	if udp {
		args = append(args, "--wf-udp-out=443")
	}
	return append(args, "--wf-dup-check")
}

func Check(ctx context.Context, path string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	h, e := exec.CommandContext(ctx, path, "--help").CombinedOutput()
	if e != nil && len(h) == 0 {
		return fmt.Errorf("движок: %w", e)
	}
	for _, feature := range []string{"--dry-run", "--lua-desync=", "--lua-init=", "--hostlist=", "--new", "--pidfile=", "--wf-tcp-out="} {
		if !strings.Contains(string(h), feature) {
			return fmt.Errorf("несовместимый winws: нет параметра %s; требуется winws2", feature)
		}
	}
	a := append([]string{"--dry-run"}, args...)
	b, e := exec.CommandContext(ctx, path, a...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("проверка параметров winws2: %w: %s", e, b)
	}
	return nil
}

// Trusted rejects missing paths and any symlink component; Windows has no root owner.
func Trusted(path string) error {
	for p := filepath.Clean(path); ; {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("в привилегированном пути запрещена символическая ссылка: %s", p)
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return nil
}
