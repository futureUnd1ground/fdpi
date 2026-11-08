package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountingCreateAndPreserve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "70-fdpi.conf")
	if e := writeAccountingFile(path); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(before), accountingKey+"=1\n") {
		t.Fatal(string(before))
	}
	if e = writeAccountingFile(path); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("existing settings changed")
	}
	custom := []byte("# local settings\n" + accountingKey + "=0\nnet.ipv4.ip_forward=1\n")
	if e = os.WriteFile(path, custom, 0600); e != nil {
		t.Fatal(e)
	}
	if e = writeAccountingFile(path); e == nil {
		t.Fatal("conflicting file overwritten")
	}
	after, _ = os.ReadFile(path)
	if string(after) != string(custom) {
		t.Fatal("custom settings destroyed")
	}
	// Last assignment wins; an earlier =1 must not hide a later =0.
	if e = os.WriteFile(path, []byte(accountingKey+"=1\n"+accountingKey+"=0\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = writeAccountingFile(path); e == nil {
		t.Fatal("conflicting duplicate accepted")
	}
}
func TestAccountingRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "custom.conf")
	link := filepath.Join(dir, "70-fdpi.conf")
	if e := os.WriteFile(target, []byte("untouched"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	if e := writeAccountingFile(link); e == nil {
		t.Fatal("symlink accepted")
	}
	b, e := os.ReadFile(target)
	if e != nil || string(b) != "untouched" {
		t.Fatalf("%s %v", b, e)
	}
}
