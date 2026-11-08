package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestControlRequiresExplicitReplacement(t *testing.T) {
	var calls []string
	cmd := func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "is-active" {
			return "active", nil
		}
		return "", nil
	}
	e := control(context.Background(), "start", false, cmd)
	if !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "stop ") || strings.HasPrefix(c, "reset-failed ") {
			t.Fatal("changed a service without consent", calls)
		}
	}
}
func TestControlRestoresForeignServicesOnFailedStart(t *testing.T) {
	var calls []string
	cmd := func(_ context.Context, args ...string) (string, error) {
		c := strings.Join(args, " ")
		calls = append(calls, c)
		if args[0] == "is-active" {
			return "active", nil
		}
		if c == "start fdpi.service" {
			return "", errors.New("engine failed")
		}
		return "", nil
	}
	if e := control(context.Background(), "start", true, cmd); e == nil {
		t.Fatal("start failure hidden")
	}
	joined := strings.Join(calls, "\n")
	for _, expected := range []string{"stop zapret.service", "stop zapret2.service", "reset-failed fdpi.service", "stop fdpi.service", "start zapret.service", "start zapret2.service"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %s: %s", expected, joined)
		}
	}
	if strings.Index(joined, "reset-failed fdpi.service") > strings.Index(joined, "start fdpi.service") {
		t.Fatal("start limit not reset before start")
	}
}
func TestEnableReplacementDoesNotStopRunningForeignServices(t *testing.T) {
	var calls []string
	cmd := func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "is-enabled" {
			return "enabled", nil
		}
		return "", nil
	}
	if e := control(context.Background(), "enable", true, cmd); e != nil {
		t.Fatal(e)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "stop ") {
			t.Fatal("enable stopped active service")
		}
	}
}
