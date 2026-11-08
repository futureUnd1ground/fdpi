package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futureUnd1ground/fdpi/internal/service"
)

// operationCommand повторно запускает fdpi.exe с аргументами команды;
// повышение прав (UAC) запросит сама команда через ShellExecute runas.
func operationCommand(args []string) *exec.Cmd {
	exe, e := os.Executable()
	if e != nil {
		exe = "fdpi.exe"
	}
	return exec.Command(exe, args...)
}

// recentLogs читает последние записи журнала движка из файла.
func recentLogs(ctx context.Context) (string, error) {
	b, e := os.ReadFile(service.LogPath())
	if e != nil {
		if os.IsNotExist(e) {
			return "журнал пуст; служба ещё не запускалась", nil
		}
		return "", e
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > 80 {
		lines = lines[len(lines)-80:]
	}
	return strings.Join(lines, "\n"), nil
}

// engineLabel — короткое название механизма перехвата для страницы «Сеть».
func engineLabel() string { return "WinDivert" }
