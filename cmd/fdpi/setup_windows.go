// Windows: мастер первого запуска настраивает службу FDPI через SCM —
// сборка из исходников и пакетный менеджер не требуются.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"github.com/futureUnd1ground/fdpi/internal/tui"
)

// componentCheck — проверка наличия файла комплекта рядом с fdpi.exe.
type componentCheck struct {
	name string
	path string
	ok   bool
}

// checkComponents проверяет движок winws2, драйвер WinDivert и Lua-библиотеки.
func checkComponents() []componentCheck {
	exe, e := os.Executable()
	if e != nil {
		exe = "fdpi.exe"
	}
	dir := filepath.Dir(exe)
	luaDir := filepath.Join(dir, "lua")
	checks := []componentCheck{
		{"Движок winws2.exe", filepath.Join(dir, "winws2.exe"), false},
		{"WinDivert.dll", filepath.Join(dir, "WinDivert.dll"), false},
		{"Драйвер WinDivert64.sys", filepath.Join(dir, "WinDivert64.sys"), false},
		{"lua\\zapret-lib.lua", filepath.Join(luaDir, "zapret-lib.lua"), false},
		{"lua\\zapret-antidpi.lua", filepath.Join(luaDir, "zapret-antidpi.lua"), false},
	}
	for i := range checks {
		st, e := os.Stat(checks[i].path)
		checks[i].ok = e == nil && !st.IsDir()
	}
	return checks
}

func missingComponents(checks []componentCheck) []string {
	var out []string
	for _, c := range checks {
		if !c.ok {
			out = append(out, c.name)
		}
	}
	return out
}

// applyDefaultStrategy назначает выбранную стратегию всем включённым профилям.
func applyDefaultStrategy(c *config.Config, strategy string) {
	for i := range c.Profiles {
		if c.Profiles[i].Enabled {
			c.Profiles[i].Strategy = strategy
		}
	}
}

func runSetup(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("на Windows мастер не принимает параметров; справка: fdpi --help")
	}
	if e := os.MkdirAll(filepath.Dir(config.UserPath()), 0700); e != nil {
		return e
	}
	lock, e := service.Lock(filepath.Join(filepath.Dir(config.UserPath()), "setup.lock"))
	if e != nil {
		return fmt.Errorf("мастер уже открыт: закройте прежнее окно установки и снова выполните fdpi: %w", e)
	}
	unlock := func() {
		if lock != nil {
			service.Unlock(lock)
			lock = nil
		}
	}
	defer unlock()
	m := newWinSetupModel(ctx)
	result, e := runWinSetupTUI(ctx, m)
	unlock()
	if e != nil {
		if errors.Is(e, errSetupCancelled) {
			fmt.Println("\nМастер остановлен. Уже выполненные действия сохранены. Продолжить: fdpi setup.")
			return nil
		}
		return e
	}
	if result {
		c, e := userConfig()
		if e != nil {
			return e
		}
		return tui.Run(c, config.UserPath())
	}
	return nil
}
