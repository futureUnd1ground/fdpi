package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// isRoot сообщает, запущен ли процесс от root.
func isRoot() bool { return os.Geteuid() == 0 }

// tuiForbiddenForRoot запрещает интерфейс от root (на Linux sudo не нужен).
func tuiForbiddenForRoot() bool { return true }

// elevated выполняет команду с правами root через sudo.
func elevated(ctx context.Context, args ...string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if isRoot() {
		return run(ctx, args)
	}
	c := exec.CommandContext(ctx, "sudo", append([]string{"--", exe}, args...)...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// updateHint — инструкция по обновлению из исходников.
func updateHint() string {
	return "В каталоге исходников: make check && make build && sudo make install\nЕсли проект находится в git, сначала проверьте обновления и выполните git pull --ff-only.\nДвижок: make engine собирает закреплённую версию. Перед её сменой прочитайте docs/ENGINE.md.\nЗагрузка и запуск удалённого кода автоматически не выполняются."
}

func adminWindowsSetup(ctx context.Context) error {
	return fmt.Errorf("команда доступна только на Windows")
}

func adminWindowsRemove(ctx context.Context) error {
	return fmt.Errorf("команда доступна только на Windows")
}
