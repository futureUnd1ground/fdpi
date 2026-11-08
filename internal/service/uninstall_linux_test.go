package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUninstallPreservesSettingsAndUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	write := func(name string) {
		p := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("preserve"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range installedFiles {
		write(p)
	}
	keep := []string{"/etc/fdpi/config.json", "/etc/sysctl.d/70-fdpi.conf", "/home/user/.config/fdpi/config.json", "/usr/lib/fdpi/unrelated"}
	for _, p := range keep {
		write(p)
	}
	var events []string
	command := func(_ context.Context, args ...string) (string, error) {
		events = append(events, args[0])
		return "", nil
	}
	cleanup := func(context.Context) error { events = append(events, "cleanup"); return nil }
	if e := uninstall(context.Background(), root, command, cleanup); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(events, []string{"disable", "cleanup", "daemon-reload"}) {
		t.Fatal(events)
	}
	for _, p := range installedFiles {
		if _, e := os.Lstat(filepath.Join(root, p)); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("installed file remains", p, e)
		}
	}
	for _, p := range keep {
		if _, e := os.Stat(filepath.Join(root, p)); e != nil {
			t.Fatal("unrelated file lost", p, e)
		}
	}
}
func TestUninstallDoesNotDeleteOnStopOrCleanupFailure(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		root := t.TempDir()
		p := filepath.Join(root, "usr/lib/systemd/system/fdpi.service")
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte("unit"), 0600)
		command := func(context.Context, ...string) (string, error) {
			if !failCleanup {
				return "", errors.New("stop failed")
			}
			return "", nil
		}
		cleanup := func(context.Context) error { return errors.New("cleanup failed") }
		if e := uninstall(context.Background(), root, command, cleanup); e == nil {
			t.Fatal("failure ignored")
		}
		if _, e := os.Stat(p); e != nil {
			t.Fatal("deleted after failure")
		}
	}
}
