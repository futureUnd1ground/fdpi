package main

import (
	"context"
	"os"
	"os/exec"
)

// runLogs показывает журнал службы через journalctl.
func runLogs(ctx context.Context, args []string) error {
	a := []string{"-u", "fdpi.service", "--no-pager", "-n", "200"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--follow":
			a = append(a, "--follow")
		case "--grep":
			i++
			if i >= len(args) {
				return errGrepArg()
			}
			a = append(a, "--grep", args[i])
		case "--category":
			i++
			if i >= len(args) {
				return errCategoryArg()
			}
			switch args[i] {
			case "err", "warning", "info":
				a = append(a, "--priority", args[i])
			default:
				return errBadCategory()
			}
		default:
			return errUnknownLogArg()
		}
	}
	c := exec.CommandContext(ctx, "/usr/bin/journalctl", a...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
