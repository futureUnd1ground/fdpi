package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/diagnostics"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"github.com/futureUnd1ground/fdpi/internal/tui"
)

var errSetupCancelled = errors.New("мастер остановлен; уже завершённые шаги сохранены")

type winInstallStep struct {
	title, description string
	action             func() (string, error)
}

type winSetupModel struct {
	ctx                 context.Context
	cfg                 config.Config
	components          []componentCheck
	page, cursor        int
	width, height       int
	strategy            int // курсор в списке стратегий
	steps               []winInstallStep
	current             int
	completed           []string
	err, detail, probes string
	loaded, busy        bool
	openApp             bool
}

func newWinSetupModel(ctx context.Context) winSetupModel {
	return winSetupModel{ctx: ctx, width: 80, height: 26}
}

// optionCount — число позиций на странице выбора сервисов и стратегии.
func (m winSetupModel) optionCount() int {
	return len(m.cfg.Profiles) + len(m.cfg.Strategies) + 2
}

// plan собирает шаги установки.
func (m *winSetupModel) plan() error {
	enabled := false
	for _, p := range m.cfg.Profiles {
		if p.Enabled {
			enabled = true
		}
	}
	if !enabled {
		return fmt.Errorf("отметьте хотя бы один сервис")
	}
	strategy := m.cfg.Strategies[m.strategy].ID
	cfg := m.cfg
	m.steps = []winInstallStep{
		{title: "Сохранить настройки", description: "Записать выбранные сервисы и стратегию в пользовательский черновик.", action: func() (string, error) {
			applyDefaultStrategy(&cfg, strategy)
			if e := cfg.Validate(); e != nil {
				return "", e
			}
			m.cfg = cfg
			return "Настройки сохранены", config.Save(config.UserPath(), cfg)
		}},
		{title: "Установить и запустить службу", description: "Регистрация службы fdpi с автозапуском, применение конфигурации в ProgramData\\FDPI и запуск обработки. Один запрос UAC.", action: func() (string, error) {
			if e := service.EnsureStateDir(); e != nil {
				return "", e
			}
			return "Служба установлена и запущена", elevated(m.ctx, "windows-setup")
		}},
	}
	m.current = 0
	m.completed = nil
	return nil
}

// probeEnabled проверяет первый домен каждого включённого профиля.
func (m *winSetupModel) probeEnabled() (string, error) {
	var b strings.Builder
	ok := true
	for _, p := range m.cfg.Profiles {
		if !p.Enabled || len(p.Domains) == 0 {
			continue
		}
		r := diagnostics.Probe(m.ctx, p.Domains[0])
		fmt.Fprintf(&b, "[%s] %s (%s): ", map[bool]string{true: "OK", false: "ОШИБКА"}[r.Reachable], p.Title(), p.Domains[0])
		if r.Reachable {
			fmt.Fprintf(&b, "HTTP %d, %d мс\n", r.HTTP, r.Millis)
		} else {
			fmt.Fprintf(&b, "%s (%s)\n", r.Error, r.Stage)
		}
		ok = ok && r.Reachable
	}
	if b.Len() == 0 {
		return "Нет включённых профилей с доменами.", nil
	}
	if !ok {
		return b.String(), fmt.Errorf("часть проверок не пройдена; журнал: fdpi logs")
	}
	return b.String(), nil
}

type winSetupLoaded struct {
	cfg        config.Config
	components []componentCheck
	err        error
}
type winStepDone struct {
	text string
	err  error
}

func (m winSetupModel) Init() tea.Cmd {
	return func() tea.Msg {
		c, e := userConfig()
		return winSetupLoaded{cfg: c, components: checkComponents(), err: e}
	}
}
func (m winSetupModel) executeCurrent() tea.Cmd {
	s := m.steps[m.current]
	return func() tea.Msg {
		text, e := s.action()
		return winStepDone{text: text, err: e}
	}
}
func (m winSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
	case winSetupLoaded:
		m.loaded = true
		m.cfg = x.cfg
		m.components = x.components
		if x.err != nil {
			m.err = x.err.Error()
		}
	case winStepDone:
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
			text, e := m.probeEnabled()
			m.probes = text
			m.page = 3
			if e != nil {
				m.err = e.Error()
			}
		}
	case tea.KeyMsg:
		k := x.String()
		if k == "ctrl+c" || k == "q" {
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		switch m.page {
		case 0:
			switch k {
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
			case "down", "j":
				m.cursor = min(1, m.cursor+1)
			case "enter":
				if m.cursor == 1 {
					return m, tea.Quit
				}
				if !m.loaded {
					m.err = "Проверка компонентов ещё выполняется"
					return m, nil
				}
				if missing := missingComponents(m.components); len(missing) > 0 {
					m.err = "Не хватает файлов: " + strings.Join(missing, ", ") + ". Распакуйте полный комплект FDPI в один каталог."
					return m, nil
				}
				m.page = 1
				m.cursor = 0
				m.err = ""
			}
		case 1:
			count := m.optionCount()
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
				s := len(m.cfg.Strategies)
				switch {
				case m.cursor < n:
					m.cfg.Profiles[m.cursor].Enabled = !m.cfg.Profiles[m.cursor].Enabled
				case m.cursor < n+s:
					if k == "enter" {
						m.strategy = m.cursor - n
					}
				case m.cursor == n+s: // Установить и запустить
					if k != "enter" {
						break
					}
					if e := m.plan(); e != nil {
						m.err = e.Error()
					} else {
						m.page = 2
						m.err = ""
					}
				default: // Назад
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
		case 3:
			if k == "enter" {
				m.openApp = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m winSetupModel) options() []string {
	var out []string
	box := func(b bool) string {
		if b {
			return "[✓] "
		}
		return "[ ] "
	}
	for _, p := range m.cfg.Profiles {
		out = append(out, box(p.Enabled)+p.Title())
	}
	for i, s := range m.cfg.Strategies {
		mark := "( ) "
		if i == m.strategy {
			mark = "(●) "
		}
		out = append(out, mark+"стратегия по умолчанию: "+s.Name)
	}
	out = append(out, "[ Установить и запустить ]", "[ Назад ]")
	return out
}

func (m winSetupModel) View() string {
	w := max(20, m.width-6)
	var b strings.Builder
	b.WriteString("Мастер настраивает службу Windows; установка отдельных пакетов не требуется.\n\n")
	switch m.page {
	case 0:
		b.WriteString("Компоненты рядом с fdpi.exe:\n")
		for _, c := range m.components {
			mark := "[✓] "
			if !c.ok {
				mark = "[✗] "
			}
			b.WriteString(mark + c.name + "\n")
		}
		if !m.loaded {
			b.WriteString("Проверка…\n")
		}
		b.WriteString("\n")
		for i, label := range []string{"Продолжить", "Выйти"} {
			prefix := "  "
			if m.cursor == i {
				prefix = "▸ "
			}
			b.WriteString(prefix + label + "\n")
		}
	case 1:
		b.WriteString("Пробел — включить сервис; Enter — выбрать стратегию по умолчанию.\nВыбранная стратегия назначается всем включённым сервисам.\n\n")
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
		if m.busy {
			b.WriteString("Выполняется…\n")
		} else {
			b.WriteString("Enter — выполнить / повторить этот шаг.\n")
		}
		for _, done := range m.completed {
			b.WriteString("✓ " + done + "\n")
		}
		if m.detail != "" {
			b.WriteString("\n" + m.detail)
		}
	case 3:
		b.WriteString("Проверка доступа к включённым сервисам:\n\n" + m.probes + "\n")
		b.WriteString("Проверка не подтверждает видео, звонки и эффективность обхода DPI.\n")
		b.WriteString("\nEnter — завершить установку")
	}
	if m.err != "" {
		b.WriteString("\nОшибка: " + m.err + "\n")
	}
	lines := strings.Split(ansi.Wrap(b.String(), w, ""), "\n")
	if len(lines) > max(5, m.height-2) {
		lines = lines[:max(5, m.height-3)]
		lines = append(lines, ansi.Truncate(m.err, w, "…"))
	}
	return tui.Frame(m.width, m.height, tui.Theme("FDPI"), []string{"① Компоненты", "② Сервисы", "③ Установка", "④ Готово"}, min(m.page, 3), "Установка и настройка", "МАСТЕР", strings.Join(lines, "\n"), "[↑↓] Выбор  [Space] Флажок  [Enter] Продолжить  [Q] Выход")
}

func runWinSetupTUI(ctx context.Context, m winSetupModel) (bool, error) {
	result, e := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if e != nil {
		return false, e
	}
	fin := result.(winSetupModel)
	return fin.openApp, nil
}
