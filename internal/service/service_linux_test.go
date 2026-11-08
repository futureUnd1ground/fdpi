package service

import (
	"context"
	"errors"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExclusiveLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	f, e := Lock(p)
	if e != nil {
		t.Fatal(e)
	}
	defer Unlock(f)
	if g, e := Lock(p); e == nil {
		Unlock(g)
		t.Fatal("duplicate lock accepted")
	}
}
func TestNetnsSupervision(t *testing.T) {
	if os.Getenv("FDPI_NETNS_TEST") != "1" {
		t.Skip("requires isolated namespace")
	}
	a, _ := os.Readlink("/proc/self/ns/net")
	b := os.Getenv("FDPI_HOST_NETNS")
	if b == "" || a == b {
		t.Fatal("host namespace refused")
	}
	p := os.Getenv("FDPI_TEST_ENGINE")
	if p == "" {
		t.Fatal("real engine required")
	}
	if e := os.WriteFile("/proc/sys/net/netfilter/nf_conntrack_acct", []byte("1"), 0600); e != nil {
		t.Fatal(e)
	}
	c := config.Default()
	c.Profiles[0].Enabled = true
	for _, crash := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		dir := t.TempDir()
		go func() { done <- Supervise(ctx, c, p, dir) }()
		ready := false
		for i := 0; i < 200; i++ {
			select {
			case e := <-done:
				cancel()
				t.Fatalf("early exit: %v", e)
			default:
			}
			yes, _ := network.Owned(context.Background())
			if yes {
				ready = true
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !ready {
			cancel()
			<-done
			t.Fatal("no active rules")
		}
		if crash {
			b, e := os.ReadFile(filepath.Join(dir, "nfqws.pid"))
			if e != nil {
				cancel()
				t.Fatal(e)
			}
			pid, e := strconv.Atoi(strings.TrimSpace(string(b)))
			if e != nil {
				cancel()
				t.Fatal(e)
			}
			if e = syscall.Kill(pid, syscall.SIGKILL); e != nil {
				cancel()
				t.Fatal(e)
			}
		} else {
			cancel()
		}
		select {
		case e := <-done:
			if crash && e == nil {
				t.Fatal("engine crash not reported")
			}
			if !crash && e != nil {
				t.Fatal(e)
			}
		case <-time.After(8 * time.Second):
			cancel()
			t.Fatal("shutdown timeout")
		}
		cancel()
		if yes, e := network.Owned(context.Background()); e != nil || yes {
			t.Fatalf("stale rules: %t %v", yes, e)
		}
	}
	bad := c
	bad.Strategies[0].TCP = "shell"
	if e := Supervise(context.Background(), bad, p, t.TempDir()); e == nil {
		t.Fatal("bad strategy accepted")
	}
	if e := Supervise(context.Background(), c, "/nonexistent/nfqws", t.TempDir()); e == nil {
		t.Fatal("missing engine accepted")
	}
	if _, e := exec.LookPath("nft"); e != nil {
		t.Fatal(e)
	}
}

func TestFailedApplyRestoresConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	backup := filepath.Join(dir, "previous.json")
	old := config.Default()
	if e := config.Save(path, old); e != nil {
		t.Fatal(e)
	}
	changed := config.Default()
	changed.Profiles[0].Enabled = true
	restored := false
	e := commit(path, backup, changed, true, func() error { return errors.New("engine restart failed") }, func() error { restored = true; return nil })
	if e == nil || !restored {
		t.Fatal("rollback was not executed")
	}
	result, e := config.Load(path)
	if e != nil || result.Profiles[0].Enabled {
		t.Fatalf("original config not restored: %v", e)
	}
	if e := commit(path, backup, changed, false, func() error { t.Fatal("stopped service restarted"); return nil }, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	result, e = config.Load(path)
	if e != nil || !result.Profiles[0].Enabled {
		t.Fatal("offline commit failed")
	}
}
