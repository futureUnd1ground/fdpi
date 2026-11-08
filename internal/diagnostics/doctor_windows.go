package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/service"
)

func Doctor(ctx context.Context) []Check {
	var out []Check
	add := func(n string, e error, d string) {
		if e != nil {
			d = e.Error()
		}
		out = append(out, Check{n, e == nil, d})
	}
	_, e := os.Stat(engine.Path)
	add("Файл движка", e, engine.Path)
	for _, name := range []string{"WinDivert.dll", "WinDivert64.sys"} {
		_, e := os.Stat(filepath.Join(filepath.Dir(engine.Path), name))
		add("Компонент "+name, e, "драйвер WinDivert рядом с winws2.exe")
	}
	c, e := config.Load(service.SystemConfig)
	add("Системные настройки", e, service.SystemConfig)
	if e == nil {
		dir, e := os.MkdirTemp("", "fdpi-doctor-")
		if e == nil {
			defer os.RemoveAll(dir)
			a, _, _, ae := engine.Args(c, dir)
			if ae == nil {
				ae = engine.Check(ctx, engine.Path, a)
			}
			add("Совместимость движка", ae, "параметры приняты реальным движком через --dry-run")
		}
	}
	add("Права администратора", func() error {
		if !service.IsAdmin() {
			return fmt.Errorf("обычный пользователь — это нормально для интерфейса; административные действия запросят повышение прав")
		}
		return nil
	}(), "администратор")
	_, e = net.DefaultResolver.LookupHost(ctx, "example.com")
	add("DNS", e, "адрес example.com получен")
	s := service.State(ctx)
	out = append(out, Check{"Служба FDPI", s.Detail == "" && s.Installed, s.StateText()})
	return out
}

// requireRoot проверяет полномочия для подбора стратегий (lab).
func requireRoot() error {
	if !service.IsAdmin() {
		return fmt.Errorf("для подбора нужны права администратора и установленная служба FDPI")
	}
	return nil
}

// restoreService применяет действие к службе FDPI при восстановлении после подбора.
func restoreService(ctx context.Context, action string) error {
	return service.Control(ctx, action, false)
}
