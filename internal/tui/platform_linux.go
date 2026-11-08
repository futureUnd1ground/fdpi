package tui

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/futureUnd1ground/fdpi/internal/network"
)

// operationCommand выполняет административную команду FDPI через sudo.
func operationCommand(args []string) *exec.Cmd {
	return exec.Command("sudo", append([]string{"--", "/usr/bin/fdpi"}, args...)...)
}

// recentLogs читает последние записи журнала службы через journalctl.
func recentLogs(ctx context.Context) (string, error) {
	b, e := exec.CommandContext(ctx, "/usr/bin/journalctl", "-u", "fdpi.service", "-n", "80", "--no-pager", "-o", "short").CombinedOutput()
	return string(b), e
}

// engineLabel — короткое название механизма перехвата для страницы «Сеть».
func engineLabel() string { return fmt.Sprintf("NFQUEUE %d", network.Queue) }
