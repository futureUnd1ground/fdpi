package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrConflict denotes a permanent conflict that restarting the daemon cannot fix.
var ErrConflict = errors.New("конфликт сетевых движков")
var otherUnits = []string{"zapret.service", "zapret2.service"}

func OtherActive(ctx context.Context) []string {
	var out []string
	for _, unit := range otherUnits {
		state, _ := Command(ctx, "is-active", unit)
		if state == "active" || state == "activating" {
			out = append(out, unit)
		}
	}
	return out
}

// Control only replaces foreign services on an explicit --replace request.
func Control(ctx context.Context, action string, replace bool) error {
	e := control(ctx, action, replace, Command)
	if e != nil && (action == "start" || action == "restart") && !errors.Is(e, ErrConflict) {
		inspect, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		b, _ := exec.CommandContext(inspect, "/usr/bin/journalctl", "-u", "fdpi.service", "-n", "15", "--no-pager", "-o", "cat").CombinedOutput()
		if len(b) > 0 {
			return fmt.Errorf("%w\nПоследние записи FDPI:\n%s", e, b)
		}
	}
	return e
}
func control(ctx context.Context, action string, replace bool, command func(context.Context, ...string) (string, error)) error {
	var foreign []string
	if action == "start" || action == "restart" {
		for _, unit := range otherUnits {
			state, _ := command(ctx, "is-active", unit)
			if state == "active" || state == "activating" {
				foreign = append(foreign, unit)
			}
		}
	}
	if action == "enable" {
		for _, unit := range otherUnits {
			state, _ := command(ctx, "is-enabled", unit)
			if state == "enabled" {
				foreign = append(foreign, unit)
			}
		}
	}
	if len(foreign) > 0 && !replace {
		return fmt.Errorf("%w: %s. Откройте fdpi и подтвердите замену либо выполните fdpi %s --replace; чужие службы автоматически не изменены", ErrConflict, strings.Join(foreign, ", "), action)
	}
	change := "stop"
	undo := "start"
	if action == "enable" {
		change = "disable"
		undo = "enable"
	}
	var changed []string
	rollback := func(cause error) error {
		restore, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if action == "start" || action == "restart" {
			_, e := command(restore, "stop", "fdpi.service")
			cause = errors.Join(cause, e)
		}
		for _, u := range changed {
			_, e := command(restore, undo, u)
			cause = errors.Join(cause, e)
		}
		return cause
	}
	for _, unit := range foreign {
		changed = append(changed, unit)
		if _, e := command(ctx, change, unit); e != nil {
			return rollback(e)
		}
	}
	if action == "start" || action == "restart" {
		if _, e := command(ctx, "reset-failed", "fdpi.service"); e != nil {
			return rollback(e)
		}
	}
	if _, e := command(ctx, action, "fdpi.service"); e != nil {
		return rollback(e)
	}
	return nil
}
