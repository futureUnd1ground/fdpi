package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/ansi"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/diagnostics"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"github.com/futureUnd1ground/fdpi/internal/tui"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type installStep struct {
	title, description string
	command            *setupCommand
	action             func() (string, error)
	optional           bool
}
type setupData struct {
	cfg     config.Config
	foreign []string
	err     error
}
type installDone struct {
	text string
	err  error
}
type setupModel struct {
	ctx                                  context.Context
	source                               textinput.Model
	sourceFocus                          bool
	cfg                                  config.Config
	foreign                              []string
	persist, autostart, replace, probe   bool
	page, cursor, current, width, height int
	steps                                []installStep
	completed                            []string
	err, detail                          string
	loaded, busy, openApp                bool
}

func newSetupModel(ctx context.Context, source string) setupModel {
	input := textinput.New()
	input.SetValue(source)
	input.CharLimit = 4096
	input.Width = 60
	return setupModel{ctx: ctx, source: input, width: 80, height: 26}
}
func (m setupModel) Init() tea.Cmd {
	return func() tea.Msg {
		c, e := userConfig()
		return setupData{cfg: c, err: e, foreign: service.OtherActive(m.ctx)}
	}
}
func (m *setupModel) options() []string {
	var out []string
	box := func(b bool) string {
		if b {
			return "[✓] "
		}
		return "[ ] "
	}
	for _, p := range m.cfg.Profiles {
		name := p.Title()
		suffix := ""
		if len(p.Domains) == 0 {
			suffix = " (сначала добавьте домены в основном TUI)"
		}
		out = append(out, box(p.Enabled)+name+suffix)
	}
	out = append(out, box(m.persist)+"Сохранять учёт conntrack после перезагрузки", box(m.autostart)+"Запускать FDPI вместе с системой", box(m.probe)+"Проверить HTTPS после запуска")
	label := "Разрешить замену zapret / zapret2"
	if len(m.foreign) > 0 {
		label += " (работают: " + strings.Join(m.foreign, ", ") + ")"
	}
	out = append(out, box(m.replace)+label, "[ Продолжить установку ]", "[ Назад ]")
	return out
}
func (m *setupModel) plan() error {
	dir, e := filepath.Abs(strings.TrimSpace(m.source.Value()))
	if e != nil {
		return e
	}
	if e = sourceValid(dir); e != nil {
		return fmt.Errorf("укажите каталог исходников: %w", e)
	}
	missing, e := missingSetupPackages(m.ctx)
	if e != nil {
		return e
	}
	m.steps = nil
	m.completed = nil
	m.current = 0
	add := func(title, desc string, c setupCommand) {
		m.steps = append(m.steps, installStep{title: title, description: desc, command: &c})
	}
	if len(missing) > 0 {
		args := append([]string{"pacman", "-S", "--needed"}, missing...)
		add("Зависимости", "Установить только отсутствующие пакеты, сохранив совместимые замены.", sudoCommand(dir, args...))
	}
	for _, s := range []struct{ title, target string }{{"Проверки проекта", "check"}, {"Сборка FDPI", "build"}, {"Сборка nfqws2", "engine"}} {
		add(s.title, "Выполнение в "+dir, setupCommand{Program: "make", Args: []string{s.target}, Dir: dir})
	}
	add("Системная установка", "Системный конфиг сохраняется; службы этим шагом не запускаются.", sudoCommand(dir, "make", "install"))
	if !runtimeAccounting() {
		add("Учёт conntrack", "Включить net.netfilter.nf_conntrack_acct=1 в текущей системе.", sudoCommand("", "/usr/bin/fdpi", "admin", "accounting-on"))
	}
	if m.persist {
		add("Настройка после перезагрузки", "Создать 70-fdpi.conf; существующее содержимое не перезаписывается.", sudoCommand("", "/usr/bin/fdpi", "admin", "accounting-persist"))
	}
	enabled := false
	target := ""
	for _, p := range m.cfg.Profiles {
		if p.Enabled {
			enabled = true
			if target == "" && len(p.Domains) > 0 {
				target = p.Domains[0]
			}
		}
	}
	if enabled && len(m.foreign) > 0 && !m.replace {
		return fmt.Errorf("уже работают %s. Для запуска FDPI отметьте разрешение остановить их либо выключите все сервисы для установки только файлов", strings.Join(m.foreign, ", "))
	}
	if e = m.cfg.Validate(); e != nil {
		return e
	}
	if enabled {
		c := m.cfg
		m.steps = append(m.steps, installStep{title: "Сохранить выбор сервисов", description: "Записать выбранные флажки в пользовательский черновик.", action: func() (string, error) {
			return "Настройки сохранены", config.Save(config.UserPath(), c)
		}})
		add("Применить настройки", "Проверить argv движка и импортировать конфигурацию в /etc/fdpi.", sudoCommand("", "/usr/bin/fdpi", "admin", "apply", config.UserPath()))
		args := []string{"/usr/bin/fdpi", "start"}
		if m.replace {
			args = append(args, "--replace")
		}
		add("Запустить FDPI", "Если разрешена замена, чужие службы будут остановлены. При ошибке запуска будет предпринято их восстановление.", sudoCommand("", args...))
		m.steps = append(m.steps, installStep{title: "Проверить состояние службы", description: "Прочитать фактическое состояние systemd.", action: func() (string, error) {
			state := service.State(m.ctx)
			if state.Service != "active" || state.Detail != "" {
				return state.Detail, fmt.Errorf("служба не работает: %s", state.StateText())
			}
			return "systemd подтверждает: служба работает", nil
		}})
		if m.probe {
			domain := target
			m.steps = append(m.steps, installStep{title: "HTTPS-проверка", description: "Запрос к " + domain + "; не проверяет видео, звонки или эффективность обхода DPI.", optional: true, action: func() (string, error) {
				r := diagnostics.Probe(m.ctx, domain)
				if !r.Reachable || r.HTTP >= 400 {
					return diagnostics.ResultText(r), fmt.Errorf("проверка доступа не пройдена")
				}
				return diagnostics.ResultText(r), nil
			}})
		}
		if m.autostart {
			args := []string{"/usr/bin/fdpi", "enable"}
			if m.replace {
				args = append(args, "--replace")
			}
			add("Автозапуск", "При разрешённой замене автозапуск zapret/zapret2 будет отключён. Действие выбрано флажком.", sudoCommand("", args...))
		}
	} else if m.autostart {
		return fmt.Errorf("для автозапуска выберите хотя бы один сервис")
	}
	return nil
}
func (m setupModel) executeCurrent() tea.Cmd {
	s := m.steps[m.current]
	if s.command != nil {
		c := s.command
		cmd := exec.CommandContext(m.ctx, c.Program, c.Args...)
		cmd.Dir = c.Dir
		tail := &setupTail{}
		cmd.Stdout = io.MultiWriter(os.Stdout, tail)
		cmd.Stderr = io.MultiWriter(os.Stderr, tail)
		return tea.ExecProcess(cmd, func(e error) tea.Msg { return installDone{err: e, text: tail.String()} })
	}
	return func() tea.Msg { text, e := s.action(); return installDone{text: text, err: e} }
}
func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
		m.source.Width = max(15, x.Width-12)
	case setupData:
		m.loaded = true
		m.cfg = x.cfg
		m.foreign = x.foreign
		if x.err != nil {
			m.err = x.err.Error()
		}
	case installDone:
		m.busy = false
		m.detail = x.text
		if x.err != nil {
			m.err = x.err.Error()
			return m, nil
		}
		m.completed = append(m.completed, m.steps[m.current].title)
		m.current++
		m.err = ""
		if m.current == len(m.steps) {
			m.page = 3
		}
	case tea.KeyMsg:
		k := x.String()
		if k == "ctrl+c" || (k == "q" && !m.sourceFocus) {
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if m.sourceFocus {
			switch k {
			case "tab", "esc", "enter":
				m.sourceFocus = false
				m.source.Blur()
			default:
				var cmd tea.Cmd
				m.source, cmd = m.source.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		switch m.page {
		case 0:
			switch k {
			case "tab", "p":
				m.sourceFocus = true
				return m, m.source.Focus()
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
			case "down", "j":
				m.cursor = min(2, m.cursor+1)
			case "enter":
				if m.cursor == 2 {
					return m, tea.Quit
				}
				if m.cursor == 1 {
					m.openApp = true
					return m, tea.Quit
				}
				if !m.loaded {
					m.err = "Проверка системы ещё выполняется"
					return m, nil
				}
				if e := sourceValid(strings.TrimSpace(m.source.Value())); e != nil {
					m.err = "Каталог исходников не найден; Tab — изменить путь"
					return m, nil
				}
				m.page = 1
				m.cursor = 0
				m.err = ""
			}
		case 1:
			count := len(m.options())
			switch k {
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
			case "down", "j":
				m.cursor = min(count-1, m.cursor+1)
			case "esc":
				m.page = 0
				m.cursor = 0
			case " ", "enter":
				n := len(m.cfg.Profiles)
				if m.cursor < n {
					if len(m.cfg.Profiles[m.cursor].Domains) == 0 {
						m.err = "Добавьте домены в основном интерфейсе: n — новый сервис"
					} else {
						m.cfg.Profiles[m.cursor].Enabled = !m.cfg.Profiles[m.cursor].Enabled
					}
					break
				}
				switch m.cursor - n {
				case 0:
					m.persist = !m.persist
					if !m.persist {
						m.autostart = false
					}
				case 1:
					m.autostart = !m.autostart
					if m.autostart {
						m.persist = true
					}
				case 2:
					m.probe = !m.probe
				case 3:
					m.replace = !m.replace
				case 4:
					if e := m.plan(); e != nil {
						m.err = e.Error()
					} else {
						m.page = 2
						m.err = ""
					}
				case 5:
					m.page = 0
					m.cursor = 0
				}
			}
		case 2:
			if k == "enter" {
				m.busy = true
				m.err = ""
				return m, m.executeCurrent()
			}
			if k == "n" && m.steps[m.current].optional {
				m.current++
				m.err = ""
				if m.current == len(m.steps) {
					m.page = 3
				}
			}
		case 3:
			if k == "enter" {
				m.openApp = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}
func (m setupModel) View() string {
	w := max(20, m.width-6)
	var b strings.Builder
	b.WriteString("Выберите шаг и подтвердите действие.\n\n")
	switch m.page {
	case 0:
		b.WriteString("Мастер работает без root. Только выбранные действия используют sudo.\nСуществующие настройки сохраняются. Выполненные шаги не откатываются при выходе.\n\nКаталог проекта (Tab — изменить):\n" + m.source.View() + "\n\n")
		for i, label := range []string{"Начать / обновить установку", "Открыть основной интерфейс без установки", "Выйти"} {
			prefix := "  "
			if m.cursor == i {
				prefix = "▸ "
			}
			b.WriteString(prefix + label + "\n")
		}
		if len(m.foreign) > 0 {
			b.WriteString("\nДругие движки уже работают: " + strings.Join(m.foreign, ", ") + "\nИх остановка требует отдельного флажка на следующем экране.\n")
		}
	case 1:
		b.WriteString("Выберите сервисы и параметры. Новые сервисы добавляются в основном TUI.\nЗамена: остановить zapret/zapret2 перед запуском; при автозапуске отключить их автозапуск.\n\n")
		options := m.options()
		start := max(0, m.cursor-max(3, m.height-13)+1)
		for i := start; i < len(options) && i < start+max(3, m.height-13); i++ {
			prefix := "  "
			if m.cursor == i {
				prefix = "▸ "
			}
			b.WriteString(prefix + options[i] + "\n")
		}
	case 2:
		s := m.steps[m.current]
		fmt.Fprintf(&b, "Шаг %d / %d · %s\n\n%s\n\n", m.current+1, len(m.steps), s.title, s.description)
		if s.command != nil {
			b.WriteString(commandText(*s.command) + "\n")
			if s.command.Dir != "" {
				b.WriteString("Каталог: " + s.command.Dir + "\n")
			}
		}
		if m.busy {
			b.WriteString("Выполняется…\n")
		} else {
			b.WriteString("Enter — выполнить / повторить этот шаг.\n")
			if s.optional {
				b.WriteString("n — пропустить эту необязательную проверку.\n")
			}
		}
		if m.detail != "" {
			b.WriteString("\n" + m.detail)
		}
	case 3:
		b.WriteString("Выбранные шаги завершены.\n")
		for _, s := range m.completed {
			b.WriteString("✓ " + s + "\n")
		}
		b.WriteString("\nEnter — открыть основной TUI · q — выйти\nОстановка обработки: fdpi stop\n")
	}
	if m.err != "" {
		b.WriteString("\nОшибка: " + m.err + "\n")
	}
	lines := strings.Split(ansi.Wrap(b.String(), w, ""), "\n")
	if len(lines) > max(5, m.height-2) {
		lines = lines[:max(5, m.height-3)]
		lines = append(lines, ansi.Truncate(m.err, w, "…"))
	}
	return tui.Frame(m.width, m.height, tui.Theme("FDPI"), []string{"① Начало", "② Параметры", "③ Установка", "④ Готово"}, m.page, "Установка и настройка", "МАСТЕР", strings.Join(lines, "\n"), "[↑↓] Выбор  [Space] Флажок  [Enter] Продолжить  [Q] Выход")
}
func runSetupTUI(ctx context.Context, source string) (bool, error) {
	result, e := tea.NewProgram(newSetupModel(ctx, source), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if e != nil {
		return false, e
	}
	m := result.(setupModel)
	if m.openApp {
		b, _ := json.Marshal(map[string]string{"version": version})
		if e = config.Write(onboardingPath(), b, 0600); e != nil {
			return false, e
		}
	}
	return m.openApp, nil
}
