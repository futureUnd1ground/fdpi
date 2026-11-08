package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/diagnostics"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"github.com/futureUnd1ground/fdpi/internal/tui"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var errSetupCancelled = errors.New("мастер остановлен; уже завершённые шаги сохранены")

type setupCommand struct {
	Program string
	Args    []string
	Dir     string
}
type setupWizard struct {
	input   *bufio.Reader
	output  io.Writer
	execute func(context.Context, setupCommand) error
	step    int
}

func (w *setupWizard) ask(ctx context.Context, prompt, def string) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	fmt.Fprint(w.output, prompt)
	type response struct {
		line string
		err  error
	}
	received := make(chan response, 1)
	go func() { line, e := w.input.ReadString('\n'); received <- response{line, e} }()
	var line string
	var e error
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-received:
		line, e = r.line, r.err
	}
	if e != nil {
		if errors.Is(e, io.EOF) {
			return "", errSetupCancelled
		}
		return "", e
	}
	answer := strings.TrimSpace(line)
	if answer == "q" || answer == "в" || strings.EqualFold(answer, "выход") {
		return "", errSetupCancelled
	}
	if answer == "" {
		return def, nil
	}
	return answer, nil
}
func (w *setupWizard) heading(title string) {
	w.step++
	fmt.Fprintf(w.output, "\n── Шаг %d. %s ──\n", w.step, title)
}
func commandText(c setupCommand) string {
	var parts []string
	for _, s := range append([]string{c.Program}, c.Args...) {
		parts = append(parts, "'"+strings.ReplaceAll(s, "'", "'\"'\"'")+"'")
	}
	return strings.Join(parts, " ")
}
func (w *setupWizard) runCommands(ctx context.Context, title string, commands []setupCommand, alreadyReady bool) error {
	w.heading(title)
	for _, c := range commands {
		if c.Dir != "" {
			fmt.Fprintf(w.output, "Каталог: %s\n", c.Dir)
		}
		fmt.Fprintf(w.output, "  %s\n", commandText(c))
	}
	prompt := "Enter — выполнить; q — выйти: "
	if alreadyReady {
		prompt = "Компонент уже найден. Enter — пропустить; r — выполнить заново; q — выйти: "
	}
	for {
		answer, e := w.ask(ctx, prompt, "")
		if e != nil {
			return e
		}
		if alreadyReady && answer == "" {
			fmt.Fprintln(w.output, "Пропущено; существующий компонент сохранён.")
			return nil
		}
		if (alreadyReady && answer != "r") || (!alreadyReady && answer != "") {
			fmt.Fprintln(w.output, "Выберите вариант из подсказки.")
			continue
		}
		break
	}
	for _, c := range commands {
		for {
			if e := ctx.Err(); e != nil {
				return e
			}
			e := w.execute(ctx, c)
			if e == nil {
				break
			}
			fmt.Fprintf(w.output, "\nШаг не завершён: %v\nСледующие действия не выполнялись.\n", e)
			for {
				answer, ae := w.ask(ctx, "Enter — повторить эту команду; q — выйти: ", "")
				if ae != nil {
					return ae
				}
				if answer == "" {
					break
				}
				fmt.Fprintln(w.output, "Для повтора нажмите Enter, для выхода — q.")
			}
		}
	}
	fmt.Fprintln(w.output, "Готово.")
	return nil
}
func sourceValid(dir string) error {
	for _, name := range []string{"go.mod", "Makefile", "cmd/fdpi/main.go", "scripts/build-engine.sh", "scripts/install.sh", "configs/default.json"} {
		st, e := os.Stat(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("не обычный файл: %s", name)
		}
	}
	return nil
}
func findSource() string {
	var candidates []string
	if exe, e := os.Executable(); e == nil {
		if real, e := filepath.EvalSymlinks(exe); e == nil {
			candidates = append(candidates, filepath.Dir(filepath.Dir(real)))
		}
	}
	if cwd, e := os.Getwd(); e == nil {
		candidates = append(candidates, cwd)
	}
	for _, dir := range candidates {
		if sourceValid(dir) == nil {
			return dir
		}
	}
	return ""
}
func (w *setupWizard) source(ctx context.Context, dir string) (string, error) {
	for {
		if dir != "" {
			abs, e := filepath.Abs(dir)
			if e == nil {
				dir = abs
				e = sourceValid(dir)
			}
			if e == nil {
				fmt.Fprintf(w.output, "Исходники: %s\n", dir)
				return dir, nil
			}
			fmt.Fprintf(w.output, "Это не полный каталог FDPI: %v\n", e)
		}
		answer, e := w.ask(ctx, "Путь к исходникам FDPI (или q — выход): ", "")
		if e != nil {
			return "", e
		}
		if strings.HasPrefix(answer, "~/") {
			home, e := os.UserHomeDir()
			if e != nil {
				return "", e
			}
			answer = filepath.Join(home, answer[2:])
		}
		dir = answer
	}
}
func (w *setupWizard) yes(ctx context.Context, prompt string) (bool, error) {
	for {
		a, e := w.ask(ctx, prompt+" [д/Н; q — выход]: ", "н")
		if e != nil {
			return false, e
		}
		switch strings.ToLower(a) {
		case "д", "да", "y", "yes":
			return true, nil
		case "н", "нет", "n", "no":
			return false, nil
		}
		fmt.Fprintln(w.output, "Ответьте д или н.")
	}
}

var setupDependencies = []string{"base-devel", "go", "git", "pkgconf", "nftables", "libnetfilter_queue", "libnfnetlink", "libmnl", "libcap", "zlib", "lua", "sudo"}

// missingSetupPackages asks pacman to resolve dependency providers, rather than
// forcing package names that could replace CachyOS compatibility packages.
func missingSetupPackages(ctx context.Context) ([]string, error) {
	args := append([]string{"-T"}, setupDependencies...)
	output, e := exec.CommandContext(ctx, "pacman", args...).Output()
	if e != nil {
		var status *exec.ExitError
		if !errors.As(e, &status) || status.ExitCode() != 127 {
			return nil, fmt.Errorf("проверка зависимостей pacman: %w", e)
		}
	}
	return parseMissingPackages(string(output))
}
func parseMissingPackages(output string) ([]string, error) {
	allowed := map[string]bool{}
	for _, name := range setupDependencies {
		allowed[name] = true
	}
	seen := map[string]bool{}
	var missing []string
	for _, name := range strings.Fields(output) {
		if !allowed[name] {
			return nil, fmt.Errorf("pacman вернул неизвестную зависимость %q; установка остановлена", name)
		}
		if !seen[name] {
			missing = append(missing, name)
			seen[name] = true
		}
	}
	return missing, nil
}
func depsReady(ctx context.Context) bool {
	for _, name := range []string{"go", "make", "cc", "git", "pkg-config", "nft", "sudo", "systemctl"} {
		if _, e := exec.LookPath(name); e != nil {
			return false
		}
	}
	return exec.CommandContext(ctx, "pkg-config", "--exists", "libnetfilter_queue", "libnfnetlink", "libmnl", "libcap", "zlib", "lua").Run() == nil
}
func runtimeAccounting() bool {
	b, e := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct")
	return e == nil && strings.TrimSpace(string(b)) == "1"
}
func (w *setupWizard) installed(ctx context.Context) error {
	for _, path := range []string{"/usr/bin/fdpi", engine.Path} {
		if e := engine.Trusted(path); e != nil {
			return e
		}
	}
	state := service.State(ctx)
	if !state.Installed || state.Detail != "" {
		return fmt.Errorf("установленная служба не обнаружена: %s", state.Detail)
	}
	return nil
}
func sudoCommand(dir string, args ...string) setupCommand {
	return setupCommand{Program: "sudo", Args: append([]string{"--"}, args...), Dir: dir}
}
func (w *setupWizard) install(ctx context.Context, source string) error {
	fmt.Fprintln(w.output, "FDPI — интерактивная установка\nEnter подтверждает показанный шаг. q — выход на любом вопросе.\nВыполненные шаги сохраняются; после ошибки можно повторить только неудачную команду.\nПароль sudo вводится в терминале, а не в чат. Весь мастер работает без root.")
	w.heading("Проверка системы")
	for _, name := range []string{"pacman", "sudo", "systemctl"} {
		path, e := exec.LookPath(name)
		if e != nil {
			return fmt.Errorf("не найдена команда %s; мастер предназначен для CachyOS/Arch с systemd", name)
		}
		fmt.Fprintf(w.output, "[✓] %s: %s\n", name, path)
	}
	dir, e := w.source(ctx, source)
	if e != nil {
		return e
	}
	fmt.Fprintln(w.output, "Из этого каталога будут выполняться Makefile и установочные скрипты.\nСистемная установка использует /usr/bin/fdpi, /usr/lib/fdpi и /etc/fdpi.")
	missing, e := missingSetupPackages(ctx)
	if e != nil {
		return e
	}
	if len(missing) == 0 {
		w.heading("Зависимости")
		fmt.Fprintln(w.output, "[✓] Пакеты уже установлены. Совместимые замены учтены через pacman -T.")
	} else {
		fmt.Fprintln(w.output, "Будут установлены только отсутствующие зависимости. Существующие совместимые пакеты сохраняются.")
		args := append([]string{"pacman", "-S", "--needed"}, missing...)
		if e = w.runCommands(ctx, "Отсутствующие зависимости", []setupCommand{sudoCommand(dir, args...)}, false); e != nil {
			return e
		}
	}
	if !depsReady(ctx) {
		return fmt.Errorf("пакеты проверены, но инструменты или pkg-config API недоступны; проверьте pkg-config --exists libnetfilter_queue libnfnetlink libmnl zlib и повторите fdpi setup")
	}
	if e = w.runCommands(ctx, "Проверка проекта", []setupCommand{{Program: "make", Args: []string{"check"}, Dir: dir}}, false); e != nil {
		return e
	}
	if e = w.runCommands(ctx, "Сборка FDPI", []setupCommand{{Program: "make", Args: []string{"build"}, Dir: dir}}, false); e != nil {
		return e
	}
	if e = w.runCommands(ctx, "Сборка настоящего nfqws2", []setupCommand{{Program: "make", Args: []string{"engine"}, Dir: dir}}, false); e != nil {
		return e
	}
	fmt.Fprintln(w.output, "Существующий системный конфиг сохраняется. Активная служба не перезапускается этим шагом.")
	if e = w.runCommands(ctx, "Установка в систему", []setupCommand{sudoCommand(dir, "make", "install")}, false); e != nil {
		return e
	}
	if e = w.installed(ctx); e != nil {
		return fmt.Errorf("проверка установки не пройдена: %w", e)
	}
	w.heading("Учёт соединений conntrack")
	if runtimeAccounting() {
		fmt.Fprintln(w.output, "[✓] Учёт уже включён; runtime sysctl не меняем.")
	} else {
		fmt.Fprintln(w.output, "Без net.netfilter.nf_conntrack_acct=1 FDPI не применит сетевые правила.")
		if e = w.runCommands(ctx, "Включить учёт в текущей системе", []setupCommand{sudoCommand("", "/usr/bin/fdpi", "admin", "accounting-on")}, false); e != nil {
			return e
		}
		if !runtimeAccounting() {
			return fmt.Errorf("учёт conntrack не включился")
		}
	}
	persist, e := w.yes(ctx, "Сохранить учёт conntrack после перезагрузки? Существующий файл не будет перезаписан")
	if e != nil {
		return e
	}
	if persist {
		if e = w.runCommands(ctx, "Сохранение sysctl", []setupCommand{sudoCommand("", "/usr/bin/fdpi", "admin", "accounting-persist")}, false); e != nil {
			return e
		}
	}
	return w.configure(ctx, persist)
}
func (w *setupWizard) configure(ctx context.Context, persisted bool) error {
	w.heading("Выбор первого сервиса")
	c, e := userConfig()
	if e != nil {
		return e
	}
	choices := []int{}
	for i, p := range c.Profiles {
		if p.ID == "youtube" && len(p.Domains) > 0 {
			choices = append(choices, i)
		}
	}
	for i, p := range c.Profiles {
		if p.ID != "youtube" && len(p.Domains) > 0 {
			choices = append(choices, i)
		}
	}
	fmt.Fprintln(w.output, "0. Только установить файлы — не менять профили и не запрашивать запуск службы")
	for n, i := range choices {
		fmt.Fprintf(w.output, "%d. %s (%d доменов)\n", n+1, c.Profiles[i].ID, len(c.Profiles[i].Domains))
	}
	if len(choices) == 0 {
		fmt.Fprintln(w.output, "Нет профилей с доменами. Добавьте их в интерфейсе: fdpi.")
		return nil
	}
	var index int
	for {
		a, ae := w.ask(ctx, "Выберите номер [1; q — выход]: ", "1")
		if ae != nil {
			return ae
		}
		if a == "0" {
			fmt.Fprintln(w.output, "Файлы установлены. Профили и состояние службы мастер не изменял. Запуск интерфейса: fdpi")
			return nil
		}
		var n int
		if _, e = fmt.Sscanf(a, "%d", &n); e == nil && fmt.Sprint(n) == a && n > 0 && n <= len(choices) {
			index = choices[n-1]
			break
		}
		fmt.Fprintln(w.output, "Введите номер из списка.")
	}
	strategy := c.Profiles[index].Strategy
	if _, ok := c.Strategy("light"); ok && !c.Profiles[index].Enabled {
		strategy = "light"
	}
	fmt.Fprintf(w.output, "Будет включён профиль %s со стратегией %s. Остальные профили сохраняются.\n", c.Profiles[index].ID, strategy)
	yes, e := w.yes(ctx, "Сохранить этот выбор в пользовательских настройках?")
	if e != nil {
		return e
	}
	if !yes {
		fmt.Fprintln(w.output, "Выбор не сохранён. Сетевые настройки мастер не применял.")
		return nil
	}
	c.Profiles[index].Enabled = true
	c.Profiles[index].Strategy = strategy
	if e = config.Save(config.UserPath(), c); e != nil {
		return e
	}
	if e = w.runCommands(ctx, "Применение проверенных настроек", []setupCommand{sudoCommand("", "/usr/bin/fdpi", "admin", "apply", config.UserPath())}, false); e != nil {
		return e
	}
	if e = w.runCommands(ctx, "Запуск службы", []setupCommand{sudoCommand("", "/usr/bin/fdpi", "start")}, false); e != nil {
		return e
	}
	state := service.State(ctx)
	if state.Detail != "" || state.Service != "active" {
		return fmt.Errorf("служба не подтвердила запуск: %s; проверьте fdpi logs", state.StateText())
	}
	fmt.Fprintln(w.output, "[✓] systemd подтверждает: служба работает.")
	w.heading("Проверка доступа")
	target := c.Profiles[index].Domains[0]
	check, e := w.yes(ctx, "Выполнить настоящий HTTPS-запрос к "+target+"?")
	if e != nil {
		return e
	}
	if check {
		r := diagnostics.Probe(ctx, target)
		fmt.Fprint(w.output, diagnostics.ResultText(r))
		if !r.Reachable || r.HTTP >= 400 {
			fmt.Fprintln(w.output, "Есть ошибка доступа. Перед автозапуском проверьте журнал или другую стратегию.")
		}
	}
	fmt.Fprintln(w.output, "Проверка HTTPS не подтверждает видео, звонки и обход DPI. Проверьте нужное приложение самостоятельно.")
	autostart, e := w.yes(ctx, "Вы проверили сервис и хотите включить автозапуск FDPI?")
	if e != nil {
		return e
	}
	if autostart && !persisted {
		fmt.Fprintln(w.output, "Автозапуск требует учёта conntrack после перезагрузки. Мастер пока не сохранял этот параметр.")
		save, se := w.yes(ctx, "Сохранить его в /etc/sysctl.d/70-fdpi.conf перед включением автозапуска?")
		if se != nil {
			return se
		}
		if save {
			if e = w.runCommands(ctx, "Настройка conntrack для автозапуска", []setupCommand{sudoCommand("", "/usr/bin/fdpi", "admin", "accounting-persist")}, false); e != nil {
				return e
			}
		} else {
			autostart = false
			fmt.Fprintln(w.output, "Автозапуск не включён. Если sysctl уже настроен другим способом, включите его самостоятельно: fdpi enable.")
		}
	}
	if autostart {
		if e = w.runCommands(ctx, "Автозапуск", []setupCommand{sudoCommand("", "/usr/bin/fdpi", "enable")}, false); e != nil {
			return e
		}
		b, ce := exec.CommandContext(ctx, "/usr/bin/systemctl", "is-enabled", "fdpi.service").Output()
		if ce != nil || strings.TrimSpace(string(b)) != "enabled" {
			return fmt.Errorf("автозапуск не подтверждён systemd")
		}
	}
	fmt.Fprintln(w.output, "\nУстановка завершена. Интерфейс: fdpi\nОстановить обработку: fdpi stop\nВернуть настройки: fdpi rollback\nЖурнал: fdpi logs")
	return nil
}
func runSetup(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("fdpi setup", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	textMode := flags.Bool("text", false, "текстовый мастер вместо TUI")
	guide := flags.Bool("guide", false, "показать текстовую инструкцию без выполнения действий")
	source := flags.String("source", findSource(), "каталог исходников FDPI")
	flags.Usage = func() {
		fmt.Fprintln(os.Stdout, "fdpi setup — мастер установки по шагам\nfdpi setup --guide — только инструкция\nfdpi setup --source ПУТЬ — выбрать каталог исходников")
	}
	if e := flags.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("лишние аргументы; справка: fdpi setup --help")
	}
	if *guide {
		fmt.Print(installationGuide())
		return nil
	}
	if os.Geteuid() == 0 {
		return fmt.Errorf("запустите fdpi setup без sudo; мастер сам вызовет sudo только для отдельных действий")
	}
	if _, e := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS); e != nil {
		return fmt.Errorf("для мастера нужен интерактивный терминал; текстовая инструкция: fdpi setup --guide")
	}
	if e := os.MkdirAll(filepath.Dir(config.UserPath()), 0700); e != nil {
		return e
	}
	lock, e := service.Lock(filepath.Join(filepath.Dir(config.UserPath()), "setup.lock"))
	if e != nil {
		return fmt.Errorf("мастер уже открыт: нажмите q в прежнем окне установки и снова выполните fdpi: %w", e)
	}
	unlock := func() {
		if lock != nil {
			service.Unlock(lock)
			lock = nil
		}
	}
	defer unlock()
	if !*textMode {
		open, e := runSetupTUI(ctx, *source)
		unlock()
		if e != nil {
			return e
		}
		if open {
			c, e := userConfig()
			if e != nil {
				return e
			}
			return tui.Run(c, config.UserPath())
		}
		return nil
	}
	w := setupWizard{input: bufio.NewReader(os.Stdin), output: os.Stdout}
	w.execute = func(ctx context.Context, c setupCommand) error {
		cmd := exec.CommandContext(ctx, c.Program, c.Args...)
		cmd.Dir = c.Dir
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	e = w.install(ctx, *source)
	if errors.Is(e, errSetupCancelled) {
		fmt.Println("\nМастер остановлен. Уже выполненные действия сохранены. Продолжить: fdpi setup.\nЕсли обработка была запущена, она продолжится в фоне. Остановить: fdpi stop.")
		return nil
	}
	return e
}
