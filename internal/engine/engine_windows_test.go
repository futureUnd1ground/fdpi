package engine

import (
	"strings"
	"testing"

	"github.com/futureUnd1ground/fdpi/internal/config"
)

// Тесты компилируются и выполняются под GOOS=windows; под Linux здесь
// проверяется только компиляция (go vet/build с GOOS=windows).

func TestWindowsPlatformArgs(t *testing.T) {
	if got := platformArgs(false, false); len(got) != 1 || got[0] != "--wf-dup-check" {
		t.Fatal(got)
	}
	got := platformArgs(true, false)
	if len(got) != 2 || got[1] != "--wf-tcp-out=80,443" {
		t.Fatal(got)
	}
	got = platformArgs(true, true)
	if len(got) != 3 || got[2] != "--wf-udp-out=443" {
		t.Fatal(got)
	}
}

func TestWindowsArgsOrder(t *testing.T) {
	c := config.Default()
	c.Profiles[0].Enabled = true
	c.Profiles[0].Strategy = "aggressive" // TCP + QUIC
	a, tcp, udp, e := Args(c, t.TempDir())
	if e != nil || !tcp || !udp {
		t.Fatalf("%v %v %v", e, tcp, udp)
	}
	joined := strings.Join(a, " ")
	// Платформенный префикс идёт первым и до первого --new.
	firstNew := strings.Index(joined, "--new")
	if firstNew < 0 {
		t.Fatal("no groups")
	}
	prefix := joined[:firstNew]
	for _, want := range []string{"--wf-dup-check", "--wf-tcp-out=80,443", "--wf-udp-out=443", "--lua-init=@", "zapret-lib.lua", "zapret-antidpi.lua"} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("префикс без %q: %s", want, prefix)
		}
	}
	if strings.Contains(prefix, "--qnum") || strings.Contains(prefix, "--uid=") || strings.Contains(prefix, "--fwmark") {
		t.Fatalf("linux-параметры в windows-argv: %s", prefix)
	}
	// Финальная catch-all группа — в конце.
	if a[len(a)-1] != "--filter-udp=443" || a[len(a)-3] != "--new" {
		t.Fatal(a)
	}
	if !strings.HasSuffix(joined, "--new --filter-tcp=80,443 --filter-udp=443") {
		t.Fatal(joined)
	}
}
