// Package engine compiles validated configuration into nfqws arguments, never shell text.
package engine

import (
	"context"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Path = "/usr/lib/fdpi/nfqws2"

// Args compiles FDPI's restricted data model, never user-provided Lua text.
func Args(c config.Config, dir string) ([]string, bool, bool, error) {
	return ArgsForEngine(c, dir, Path)
}

func ArgsForEngine(c config.Config, dir, executable string) (args []string, tcp, udp bool, err error) {
	if err = c.Validate(); err != nil {
		return
	}
	libraries := filepath.Join(filepath.Dir(executable), "lua")
	args = []string{fmt.Sprintf("--qnum=%d", network.Queue), "--uid=0:0", "--fwmark=" + network.Mark, "--bind-fix4", "--bind-fix6",
		"--lua-init=@" + filepath.Join(libraries, "zapret-lib.lua"), "--lua-init=@" + filepath.Join(libraries, "zapret-antidpi.lua")}
	count := 0
	for _, p := range c.Profiles {
		if !p.Enabled {
			continue
		}
		s, _ := c.Strategy(p.Strategy)
		host := filepath.Join(dir, p.ID+".txt")
		if err = config.Write(host, []byte(strings.Join(p.Domains, "\n")+"\n"), 0600); err != nil {
			return
		}
		add := func(a []string) {
			if count > 0 {
				args = append(args, "--new")
			}
			args = append(args, a...)
			args = append(args, "--hostlist="+host)
			count++
		}
		if s.TCP != "" || s.HostCase {
			a := []string{"--filter-tcp=80,443", "--in-range=x"}
			if s.HostCase && s.TCP == "" {
				a = []string{"--filter-tcp=80", "--in-range=x"}
			}
			for _, mode := range strings.Split(s.TCP, ",") {
				switch mode {
				case "fake":
					suffix := ":repeats=" + strconv.Itoa(s.Repeats)
					for _, fool := range strings.Split(s.Fooling, ",") {
						switch fool {
						case "badseq":
							suffix += ":tcp_seq=-10000"
						case "md5sig":
							suffix += ":tcp_md5"
						}
					}
					if s.TTL > 0 {
						suffix += ":ip_ttl=" + strconv.Itoa(s.TTL) + ":ip6_ttl=" + strconv.Itoa(s.TTL)
					}
					a = append(a, "--payload=http_req", "--lua-desync=fake:blob=fake_default_http"+suffix,
						"--payload=tls_client_hello", "--lua-desync=fake:blob=fake_default_tls"+suffix)
				case "multisplit", "multidisorder":
					a = append(a, "--payload=http_req,tls_client_hello", "--lua-desync="+mode+":pos="+s.Split)
				}
			}
			if s.HostCase {
				a = append(a, "--payload=http_req", "--lua-desync=http_hostcase")
			}
			add(a)
			tcp = true
		}
		if s.QUIC {
			add([]string{"--filter-udp=443", "--filter-l7=quic", "--in-range=x", "--payload=quic_initial",
				"--lua-desync=fake:blob=fake_default_quic:repeats=" + strconv.Itoa(s.Repeats)})
			udp = true
		}
	}
	if count > 0 {
		args = append(args, "--new", "--filter-tcp=80,443", "--filter-udp=443")
	}
	return
}

// TrustedLibraries prevents privileged execution of writable Lua dependencies.
func TrustedLibraries() error {
	for _, name := range []string{"zapret-lib.lua", "zapret-antidpi.lua"} {
		if err := Trusted(filepath.Join(filepath.Dir(Path), "lua", name)); err != nil {
			return err
		}
	}
	return nil
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
