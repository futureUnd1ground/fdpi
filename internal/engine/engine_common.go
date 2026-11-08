// Package engine compiles validated configuration into nfqws/winws arguments, never shell text.
package engine

import (
	"github.com/futureUnd1ground/fdpi/internal/config"
	"path/filepath"
	"strconv"
	"strings"
)

// Args compiles FDPI's restricted data model, never user-provided Lua text.
func Args(c config.Config, dir string) ([]string, bool, bool, error) {
	return ArgsForEngine(c, dir, Path)
}

func ArgsForEngine(c config.Config, dir, executable string) (args []string, tcp, udp bool, err error) {
	if err = c.Validate(); err != nil {
		return
	}
	libraries := filepath.Join(filepath.Dir(executable), "lua")
	// Группы аргументов собираются отдельно; платформенный префикс (platformArgs)
	// вставляется перед ними, чтобы Linux-вывод остался побайтово прежним.
	var groups [][]string
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
			groups = append(groups, append(a, "--hostlist="+host))
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
	args = append(platformArgs(tcp, udp),
		"--lua-init=@"+filepath.Join(libraries, "zapret-lib.lua"),
		"--lua-init=@"+filepath.Join(libraries, "zapret-antidpi.lua"))
	for i, g := range groups {
		if i > 0 {
			args = append(args, "--new")
		}
		args = append(args, g...)
	}
	if len(groups) > 0 {
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
