// FDPI manages domain-oriented nfqws strategies on Linux.
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/diagnostics"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"github.com/futureUnd1ground/fdpi/internal/tui"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const version = config.ApplicationVersion

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "fdpi:", e)
		if errors.Is(e, service.ErrConflict) {
			os.Exit(78)
		}
		os.Exit(1)
	}
}
func userConfig() (config.Config, error) {
	p := config.UserPath()
	c, e := config.Load(p)
	if errors.Is(e, os.ErrNotExist) {
		c = config.Default()
		e = config.Save(p, c)
	}
	changed := false
	if e == nil {
		changed = config.LocalizeBuiltin(&c)
		changed = config.TrimUnusedStock(&c) || changed
	}
	if e == nil && changed {
		e = config.Save(p, c)
	}
	return c, e
}
func unit(ctx context.Context, action string, replace bool) error {
	if !isRoot() {
		if replace {
			return elevated(ctx, action, "--replace")
		}
		return elevated(ctx, action)
	}
	lock, e := service.Lock(service.OperationLockPath())
	if e != nil {
		return e
	}
	defer service.Unlock(lock)
	if action == "start" || action == "restart" || action == "enable" {
		if e := service.RequireCurrentBinary(ctx); e != nil {
			return e
		}
	}
	if e := service.Control(ctx, action, replace); e != nil {
		return e
	}
	return nil
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		if tuiForbiddenForRoot() && isRoot() {
			return fmt.Errorf("запускайте интерфейс без sudo; root нужен только для службы и административных действий")
		}
		st, e := os.Stdin.Stat()
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeCharDevice == 0 {
			return fmt.Errorf("для интерфейса нужен терминал; справка: fdpi --help")
		}
		if needsOnboarding() {
			return runSetup(ctx, nil)
		}
		c, e := userConfig()
		if e != nil {
			return e
		}
		return tui.Run(c, config.UserPath())
	}
	switch args[0] {
	case "tui":
		if tuiForbiddenForRoot() && isRoot() {
			return fmt.Errorf("открывайте интерфейс без sudo")
		}
		c, e := userConfig()
		if e != nil {
			return e
		}
		return tui.Run(c, config.UserPath())
	case "--version", "version":
		fmt.Println("FDPI", version)
		return nil
	case "--help", "help", "-h":
		fmt.Print(`FDPI — быстрый обход DPI
Использование: fdpi [команда]
  без команды                Открыть интерфейс на русском языке
  uninstall --yes            Удалить установленный FDPI, сохранив настройки
  setup                      TUI установки: стрелки, пробел и Enter
  tui                        Основной TUI без мастера первого запуска
  start|stop|restart          Запустить / остановить / перезапустить службу
    start --replace           Подтвердить остановку zapret/zapret2
    enable --replace          Подтвердить замену их автозапуска
  enable|disable              Включить / выключить автозапуск
  status [--json]             Состояние службы и очереди пакетов
  profiles|strategies         Сервисы / стратегии из локальных настроек
  config init|show|validate   Создать / показать / проверить настройки
  config export ПУТЬ          Сохранить пресет в файл
  config import ПУТЬ          Проверить и импортировать пресет
  config system              Показать системные настройки (через sudo)
  config enable ID on|off    Включить / выключить сервис в черновике
  config strategy ID SID     Назначить сервису стратегию
  apply                      Применить черновик; перезапустить активную службу
  rollback                   Вернуть предыдущие системные настройки
  test ДОМЕН                 Проверить DNS, TCP, сертификат TLS и HTTPS
  test-service youtube        Главная, API и изображение; --interface ИМЯ, --json
  lab СЕРВИС                 Подобрать стратегию; затем вернуть настройки
  doctor [--export ПУТЬ]     Диагностика; часть проверок требует sudo
  logs [--follow]             Журнал; --follow — новые записи в реальном времени
    [--grep ТЕКСТ] [--category err|warning|info] — поиск и уровень записей
  theme [НАЗВАНИЕ]            Показать темы / выбрать тему
  update                     Инструкция по безопасному обновлению
  service-install            Установить службу Windows и применить настройки (UAC)
  service-remove             Остановить и удалить службу Windows (UAC)
`)

		return nil
	case "start", "stop", "restart", "enable", "disable":
		replace := len(args) == 2 && args[1] == "--replace"
		if len(args) > 1 && !replace {
			return fmt.Errorf("fdpi %s [--replace]", args[0])
		}
		return unit(ctx, args[0], replace)
	case "status":
		s := service.State(ctx)
		if len(args) > 1 && args[1] == "--json" {
			fmt.Println(service.JSON(s))
		} else {
			fmt.Printf("Служба: %s\nPID процесса: %d\nОчередь %d: %s\n%s\n", s.StateText(), s.SupervisorPID, network.Queue, s.QueueText(), s.Detail)
		}
		if s.Detail != "" {
			return fmt.Errorf("не удалось получить состояние systemd")
		}
		return nil
	case "daemon":
		if !isRoot() {
			return fmt.Errorf("для запуска фоновой службы нужны права администратора")
		}
		return service.Daemon()
	case "cleanup":
		if !isRoot() {
			return fmt.Errorf("для удаления собственных сетевых правил нужны права администратора")
		}
		return network.Cleanup(ctx)
	case "apply":
		return elevated(ctx, "admin", "apply", config.UserPath())
	case "rollback":
		return elevated(ctx, "admin", "rollback")
	case "uninstall":
		if len(args) != 2 || args[1] != "--yes" {
			return fmt.Errorf("удаление требует подтверждения: fdpi uninstall --yes; настройки сохранятся")
		}
		if !isRoot() {
			if err := elevated(ctx, args...); err != nil {
				return err
			}
			return service.RemoveUserLauncher()
		}
		if err := service.Uninstall(ctx); err != nil {
			return err
		}
		fmt.Println("Удаление FDPI завершено. Настройки и профили сохранены; на Windows часть файлов удалится после перезагрузки.")
		return nil
	case "admin":
		if !isRoot() {
			return fmt.Errorf("для административных действий нужны права администратора")
		}
		if len(args) < 2 {
			return fmt.Errorf("admin apply PATH | rollback")
		}
		switch args[1] {
		case "apply":
			if len(args) != 3 {
				return fmt.Errorf("admin apply PATH")
			}
			return service.Apply(ctx, args[2])
		case "rollback":
			return service.Apply(ctx, service.PreviousConfig)
		case "accounting-on":
			if len(args) != 2 {
				return fmt.Errorf("admin accounting-on не принимает дополнительных аргументов")
			}
			return service.EnableAccounting(ctx)
		case "accounting-persist":
			if len(args) != 2 {
				return fmt.Errorf("admin accounting-persist не принимает дополнительных аргументов")
			}
			return service.PersistAccounting()
		case "windows-setup":
			if len(args) != 2 {
				return fmt.Errorf("admin windows-setup не принимает дополнительных аргументов")
			}
			return adminWindowsSetup(ctx)
		default:
			return fmt.Errorf("неизвестная административная команда")
		}
	case "windows-setup":
		return elevated(ctx, "admin", "windows-setup")
	case "service-install":
		return elevated(ctx, "admin", "windows-setup")
	case "service-remove":
		return elevated(ctx, "windows-remove")
	case "windows-remove":
		return adminWindowsRemove(ctx)
	case "profiles", "strategies":
		c, e := userConfig()
		if e != nil {
			return e
		}
		if args[0] == "profiles" {
			fmt.Println(service.JSON(c.Profiles))
		} else {
			fmt.Println(service.JSON(c.Strategies))
		}
		return nil
	case "config":
		return configure(ctx, args[1:])
	case "test":
		if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "--json") {
			return fmt.Errorf("fdpi test ДОМЕН [--json]")
		}
		r := diagnostics.Probe(ctx, args[1])
		if len(args) == 3 {
			fmt.Println(service.JSON(r))
		} else {
			fmt.Print(diagnostics.ResultText(r))
		}
		if !r.Reachable {
			return fmt.Errorf("проверка завершилась ошибкой на этапе %s", r.Stage)
		}
		return nil
	case "test-service":
		if len(args) < 2 || args[1] != "youtube" {
			return fmt.Errorf("fdpi test-service youtube [--interface ИНТЕРФЕЙС] [--json]")
		}
		iface := ""
		addresses := map[string]string{}
		asJSON := false
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--json":
				asJSON = true
			case "--resolve":
				i++
				if i >= len(args) {
					return fmt.Errorf("не указан HOST=IP")
				}
				host, ip, ok := strings.Cut(args[i], "=")
				if !ok || net.ParseIP(ip) == nil {
					return fmt.Errorf("ожидается HOST=IP")
				}
				addresses[host] = ip
			case "--interface":
				i++
				if i >= len(args) {
					return fmt.Errorf("не указан интерфейс")
				}
				iface = args[i]
			default:
				return fmt.Errorf("неизвестный параметр: %s", args[i])
			}
		}
		report := diagnostics.YouTubeResolved(ctx, iface, addresses)
		if asJSON {
			fmt.Println(service.JSON(report))
		} else {
			fmt.Print(diagnostics.YouTubeText(report))
		}
		if !report.OK {
			return fmt.Errorf("проверка YouTube выявила ошибки")
		}
		return nil
	case "lab":
		if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "--json") {
			return fmt.Errorf("fdpi lab СЕРВИС [--json]")
		}
		if !isRoot() {
			return elevated(ctx, args...)
		}
		r, e := diagnostics.Lab(ctx, args[1])
		if len(args) == 3 {
			fmt.Println(service.JSON(r))
		} else {
			fmt.Print(diagnostics.ReportText(r))
		}
		return e
	case "doctor":
		c, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		checks := diagnostics.Doctor(c)
		b := service.JSON(checks)
		if len(args) == 3 && args[1] == "--export" {
			return config.Write(args[2], []byte(b+"\n"), 0600)
		}
		if len(args) != 1 && !(len(args) == 2 && args[1] == "--json") {
			return fmt.Errorf("fdpi doctor [--json | --export ПУТЬ]")
		}
		if len(args) == 2 {
			fmt.Println(b)
		} else {
			fmt.Print(diagnostics.ChecksText(checks))
		}
		for _, check := range checks {
			if !check.OK {
				return fmt.Errorf("некоторые проверки требуют внимания; подробности указаны выше")
			}
		}
		return nil
	case "logs":
		return runLogs(ctx, args[1:])
	case "theme":
		if len(args) == 1 {
			for _, name := range tui.Themes {
				fmt.Printf("%s [%s]\n", tui.ThemeTitle(name), name)
			}
			fmt.Println("Выбрана:", tui.ThemeTitle(tui.LoadTheme()))
			return nil
		}
		name := strings.Join(args[1:], " ")
		for _, n := range tui.Themes {
			if name == n || name == tui.ThemeTitle(n) {
				return tui.SaveTheme(n)
			}
		}
		return fmt.Errorf("неизвестная тема")
	case "setup":
		return runSetup(ctx, args[1:])
	case "update":
		fmt.Println(updateHint())
		return nil
	}
	return fmt.Errorf("неизвестная команда %q; справка: fdpi --help", args[0])
}
func configure(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Println(config.UserPath())
		return nil
	}
	if args[0] == "system" {
		if !isRoot() {
			return elevated(ctx, "config", "system")
		}
		c, e := config.Load(service.SystemConfig)
		if e != nil {
			return e
		}
		fmt.Println(service.JSON(c))
		return nil
	}
	if args[0] == "init" {
		if _, e := os.Stat(config.UserPath()); e == nil {
			return fmt.Errorf("настройки уже существуют; текущий файл сохранён без изменений")
		}
		return config.Save(config.UserPath(), config.Default())
	}
	if args[0] == "import" {
		if len(args) != 2 {
			return fmt.Errorf("fdpi config import ПУТЬ")
		}
		c, e := config.Load(args[1])
		if e != nil {
			return e
		}
		return config.Save(config.UserPath(), c)
	}
	c, e := userConfig()
	if e != nil {
		return e
	}
	switch args[0] {
	case "show":
		fmt.Println(service.JSON(c))
		return nil
	case "validate":
		return c.Validate()
	case "export":
		if len(args) != 2 {
			return fmt.Errorf("fdpi config export ПУТЬ")
		}
		p, e := filepath.Abs(args[1])
		if e != nil {
			return e
		}
		return config.Save(p, c)
	case "enable", "strategy":
		if len(args) != 3 {
			return fmt.Errorf("fdpi config %s ID ЗНАЧЕНИЕ", args[0])
		}
		found := false
		for i := range c.Profiles {
			if c.Profiles[i].ID == args[1] {
				found = true
				if args[0] == "strategy" {
					c.Profiles[i].Strategy = args[2]
				} else {
					if args[2] != "on" && args[2] != "off" {
						return fmt.Errorf("укажите on — включить или off — выключить")
					}
					c.Profiles[i].Enabled = args[2] == "on"
				}
			}
		}
		if !found {
			return fmt.Errorf("неизвестный профиль")
		}
		return config.Save(config.UserPath(), c)
	}
	return fmt.Errorf("неизвестная команда настройки")
}
