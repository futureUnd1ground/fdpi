package engine

import (
	"context"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"os"
	"strings"
	"testing"
)

func TestHostlistAndCatchall(t *testing.T) {
	c := config.Default()
	c.Profiles[0].Enabled = true
	a, tcp, udp, e := Args(c, t.TempDir())
	if e != nil || !tcp || udp {
		t.Fatalf("%v %v %v", e, tcp, udp)
	}
	if !strings.Contains(strings.Join(a, " "), "--hostlist=") || a[len(a)-1] != "--filter-udp=443" {
		t.Fatal(a)
	}
}
func TestRealEngineDryRun(t *testing.T) {
	p := os.Getenv("FDPI_TEST_ENGINE")
	if p == "" {
		t.Skip("set FDPI_TEST_ENGINE to real nfqws")
	}
	c := config.Default()
	for _, s := range c.Strategies {
		t.Run(s.ID, func(t *testing.T) {
			v := c
			v.Profiles = append([]config.Profile(nil), c.Profiles...)
			v.Profiles[0].Enabled = true
			v.Profiles[0].Strategy = s.ID
			a, _, _, e := ArgsForEngine(v, t.TempDir(), p)
			if e != nil {
				t.Fatal(e)
			}
			if e = Check(context.Background(), p, a); e != nil {
				t.Fatal(e)
			}
		})
	}
}
