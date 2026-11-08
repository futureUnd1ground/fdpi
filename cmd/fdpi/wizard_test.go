package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWizardCancelNeverRunsCommands(t *testing.T) {
	for _, input := range []string{"q\n", "в\n", ""} {
		t.Run(input, func(t *testing.T) {
			called := false
			var out bytes.Buffer
			w := setupWizard{input: bufio.NewReader(strings.NewReader(input)), output: &out, execute: func(context.Context, setupCommand) error { called = true; return nil }}
			e := w.runCommands(context.Background(), "Установка", []setupCommand{{Program: "sudo", Args: []string{"make", "install"}}}, false)
			if !errors.Is(e, errSetupCancelled) || called {
				t.Fatalf("cancel=%v called=%t", e, called)
			}
		})
	}
}
func TestWizardRetriesOnlyFailedCommand(t *testing.T) {
	var calls []string
	attempt := 0
	var out bytes.Buffer
	w := setupWizard{input: bufio.NewReader(strings.NewReader("\n\n")), output: &out, execute: func(_ context.Context, c setupCommand) error {
		calls = append(calls, c.Program)
		if c.Program == "second" {
			attempt++
			if attempt == 1 {
				return errors.New("package manager failed")
			}
		}
		return nil
	}}
	if e := w.runCommands(context.Background(), "Проверка", []setupCommand{{Program: "first"}, {Program: "second"}, {Program: "third"}}, false); e != nil {
		t.Fatal(e)
	}
	if strings.Join(calls, ",") != "first,second,second,third" {
		t.Fatal(calls)
	}
}
func TestWizardReadyStepIsSkippedAndFailureStops(t *testing.T) {
	var out bytes.Buffer
	called := false
	w := setupWizard{input: bufio.NewReader(strings.NewReader("\n")), output: &out, execute: func(context.Context, setupCommand) error { called = true; return nil }}
	if e := w.runCommands(context.Background(), "Уже установлен", []setupCommand{{Program: "sudo"}}, true); e != nil || called {
		t.Fatalf("%v %t", e, called)
	}
	var calls []string
	w.input = bufio.NewReader(strings.NewReader("\nq\n"))
	w.execute = func(_ context.Context, c setupCommand) error {
		calls = append(calls, c.Program)
		return errors.New("failed")
	}
	e := w.runCommands(context.Background(), "Ошибка", []setupCommand{{Program: "first"}, {Program: "must-not-run"}}, false)
	if !errors.Is(e, errSetupCancelled) || strings.Join(calls, ",") != "first" {
		t.Fatalf("%v %v", e, calls)
	}
}
func TestWizardSourceValidationAndQuotedPaths(t *testing.T) {
	dir := t.TempDir()
	if sourceValid(dir) == nil {
		t.Fatal("empty source directory accepted")
	}
	for _, name := range []string{"go.mod", "Makefile", "cmd/fdpi/main.go", "scripts/build-engine.sh", "scripts/install.sh", "configs/default.json"} {
		p := filepath.Join(dir, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte("test"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := sourceValid(dir); e != nil {
		t.Fatal(e)
	}
	got := commandText(setupCommand{Program: "sudo", Args: []string{"--", "/path with spaces/tool", "a'$(touch /tmp/should-not-execute)"}})
	if !strings.Contains(got, `'a'"'"'$(touch /tmp/should-not-execute)'`) {
		t.Fatal(got)
	}
}

func TestMissingPackagesRejectsUnexpectedArguments(t *testing.T) {
	packages, e := parseMissingPackages("libnetfilter_queue\nlibnetfilter_queue\n")
	if e != nil || len(packages) != 1 || packages[0] != "libnetfilter_queue" {
		t.Fatalf("%v %v", packages, e)
	}
	for _, input := range []string{"--overwrite=*", "unknown-package", "libnetfilter_queue;reboot"} {
		if _, e := parseMissingPackages(input); e == nil {
			t.Fatalf("unexpected package accepted: %q", input)
		}
	}
	if missing, e := parseMissingPackages(""); e != nil || len(missing) != 0 {
		t.Fatalf("%v %v", missing, e)
	}
}

func TestPacmanPlanPreservesInstalledZlibProvider(t *testing.T) {
	if _, e := exec.LookPath("pacman"); e != nil {
		t.Skip("Arch/CachyOS pacman not available")
	}
	if e := exec.Command("pacman", "-T", "zlib").Run(); e != nil {
		t.Skip("no installed zlib provider")
	}
	missing, e := missingSetupPackages(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range missing {
		if name == "zlib" {
			t.Fatal("already satisfied zlib dependency was scheduled for replacement")
		}
	}
}
