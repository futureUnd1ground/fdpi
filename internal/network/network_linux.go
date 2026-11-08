// Package network owns exactly one nftables table, with a collision guard.
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const Table = "fdpi"
const Queue = 24000
const Mark = "0x20000000"
const Owner = "fdpi-managed-v1"

func Rules(tcp, udp bool, existing bool) string {
	var b strings.Builder
	if existing {
		b.WriteString("delete table inet fdpi\n")
	}
	b.WriteString("table inet fdpi {\n comment \"fdpi-managed-v1\"\n chain output {\n type filter hook output priority mangle; policy accept;\n meta mark & 0x20000000 != 0 return\n oifname \"lo\" return\n")
	if tcp {
		fmt.Fprintf(&b, " tcp dport { 80, 443 } ct original packets 1-20 counter queue num %d bypass\n", Queue)
	}
	if udp {
		fmt.Fprintf(&b, " udp dport 443 ct original packets 1-12 counter queue num %d bypass\n", Queue)
	}
	b.WriteString(" }\n}\n")
	return b.String()
}
func run(ctx context.Context, input string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "/usr/bin/nft", args...)
	c.Stdin = strings.NewReader(input)
	b, e := c.CombinedOutput()
	if e != nil {
		return b, fmt.Errorf("nft %v: %w: %s", args, e, b)
	}
	return b, nil
}
func Owned(ctx context.Context) (bool, error) {
	b, e := run(ctx, "", "-j", "list", "tables")
	if e != nil {
		return false, e
	}
	var v struct {
		NFT []struct {
			Table *struct {
				Family  string `json:"family"`
				Name    string `json:"name"`
				Comment string `json:"comment"`
			} `json:"table"`
		} `json:"nftables"`
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return false, e
	}
	for _, x := range v.NFT {
		if x.Table != nil && x.Table.Family == "inet" && x.Table.Name == Table {
			b, e = run(ctx, "", "-j", "list", "table", "inet", Table)
			if e != nil {
				return false, e
			}
			var detail struct {
				NFT []struct {
					Table *struct {
						Comment string `json:"comment"`
					} `json:"table"`
				} `json:"nftables"`
			}
			if e = json.Unmarshal(b, &detail); e != nil {
				return false, e
			}
			for _, d := range detail.NFT {
				if d.Table != nil && d.Table.Comment == Owner {
					return true, nil
				}
			}
			return false, fmt.Errorf("таблица inet fdpi не имеет метки FDPI; изменение чужих правил запрещено")
		}
	}
	return false, nil
}
func Apply(ctx context.Context, tcp, udp bool) error {
	b, e := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct")
	if e != nil || strings.TrimSpace(string(b)) != "1" {
		return fmt.Errorf("нужен учёт conntrack: явно включите net.netfilter.nf_conntrack_acct=1; FDPI не меняет sysctl автоматически")
	}

	yes, e := Owned(ctx)
	if e != nil {
		return e
	}
	rules := Rules(tcp, udp, yes)
	if _, e = run(ctx, rules, "--check", "-f", "-"); e != nil {
		return e
	}
	_, e = run(ctx, rules, "-f", "-")
	return e
}
func Cleanup(ctx context.Context) error {
	yes, e := Owned(ctx)
	if e != nil {
		return e
	}
	if !yes {
		return nil
	}
	_, e = run(ctx, "", "delete", "table", "inet", Table)
	return e
}
