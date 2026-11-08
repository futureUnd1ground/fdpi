package network

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRulesScope(t *testing.T) {
	r := Rules(true, true, true)
	for _, v := range []string{"flush ruleset", "delete table ip ", "drop"} {
		if strings.Contains(r, v) {
			t.Fatal(r)
		}
	}
	for _, v := range []string{"delete table inet fdpi", "queue num 24000 bypass", "meta mark & 0x20000000", "ct original packets", "hook output"} {
		if !strings.Contains(r, v) {
			t.Fatal(r)
		}
	}
}
func Isolated(t *testing.T) {
	t.Helper()
	if os.Getenv("FDPI_NETNS_TEST") != "1" {
		t.Skip("isolated netns only")
	}
	a, e := os.Readlink("/proc/self/ns/net")
	if e != nil {
		t.Fatal(e)
	}
	b := os.Getenv("FDPI_HOST_NETNS")
	if b == "" || a == b {
		t.Fatal("refusing tests without distinct recorded host namespace")
	}
}
func TestNetnsApplyCleanup(t *testing.T) {
	Isolated(t)
	ctx := context.Background()
	if e := os.WriteFile("/proc/sys/net/netfilter/nf_conntrack_acct", []byte("1"), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command("/usr/bin/nft", "add", "table", "inet", "unrelated").CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, b)
	}
	defer exec.Command("/usr/bin/nft", "delete", "table", "inet", "unrelated").Run()
	for i := 0; i < 2; i++ {
		if e := Apply(ctx, true, true); e != nil {
			t.Fatal(e)
		}
	}
	if yes, e := Owned(ctx); e != nil || !yes {
		t.Fatalf("%t %v", yes, e)
	}
	if e := Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if e := Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if e := exec.Command("/usr/bin/nft", "list", "table", "inet", "unrelated").Run(); e != nil {
		t.Fatal("foreign table removed")
	}
	if e := exec.Command("/usr/bin/nft", "add", "table", "inet", "fdpi").Run(); e != nil {
		t.Fatal(e)
	}
	defer exec.Command("/usr/bin/nft", "delete", "table", "inet", "fdpi").Run()
	if e := Cleanup(ctx); e == nil {
		t.Fatal("foreign table ownership ignored")
	}
}
func BenchmarkRules(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Rules(true, true, true)
	}
}
