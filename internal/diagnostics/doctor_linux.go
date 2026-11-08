package diagnostics

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/network"
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
	b, e := exec.CommandContext(ctx, "/usr/bin/nft", "--version").CombinedOutput()
	add("nftables", e, strings.TrimSpace(string(b)))
	_, e = network.Owned(ctx)
	add("Доступ к таблице и её принадлежность", e, "проверка таблицы без изменений выполнена")
	b, e = exec.CommandContext(ctx, "/usr/bin/nft", "-j", "list", "ruleset").CombinedOutput()
	if e == nil {
		text := string(b)
		if strings.Contains(text, "flowtable") || strings.Contains(text, "\"queue\"") {
			out = append(out, Check{"Возможные конфликты firewall", false, "найдена очередь или flowtable; проверьте nft list ruleset, включая правила самого FDPI"})
		} else {
			add("Возможные конфликты firewall", nil, "очереди и flowtable не найдены; другие взаимодействия с firewall всё же возможны")
		}
	}
	acct, ae := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct")
	if ae == nil && strings.TrimSpace(string(acct)) != "1" {
		ae = fmt.Errorf("нужно значение net.netfilter.nf_conntrack_acct=1; FDPI не меняет его автоматически")
	}
	add("Учёт соединений conntrack", ae, strings.TrimSpace(string(acct)))
	_, e = os.Stat("/proc/net/netfilter/nfnetlink_queue")
	add("Поддержка NFQUEUE в ядре", e, "доступна; может потребоваться загрузка модуля")
	b, e = os.ReadFile("/proc/self/status")
	add("Полномочия процесса в ядре", e, capLine(string(b)))
	add("Права администратора", func() error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("обычный пользователь — это нормально для интерфейса; административные действия используют sudo")
		}
		return nil
	}(), "root")
	_, e = net.DefaultResolver.LookupHost(ctx, "example.com")
	add("DNS", e, "адрес example.com получен")
	s := service.State(ctx)
	out = append(out, Check{"Служба systemd", s.Detail == "" && s.Installed, s.StateText()})
	return out
}
func capLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "CapEff:") {
			return l
		}
	}
	return "поле CapEff недоступно"
}

// requireRoot проверяет полномочия для подбора стратегий (lab).
func requireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("для подбора нужны права root и установленная служба systemd")
	}
	return nil
}

// restoreService применяет действие к службе FDPI при восстановлении после подбора.
func restoreService(ctx context.Context, action string) error {
	_, e := service.Command(ctx, action, "fdpi.service")
	return e
}
