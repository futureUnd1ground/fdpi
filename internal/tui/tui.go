// Package tui implements an unprivileged, event-driven terminal interface.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/diagnostics"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

var pages = []string{"Обзор", "Сервисы", "Стратегии", "Подбор стратегии", "Сеть", "Журнал", "Настройки"}

type refreshMsg struct {
	foreign []string
	state   service.Status
	text    string
}
type resultMsg struct {
	text string
	err  error
}
type Model struct {
	form                                *profileForm
	deleteProfile                       int
	palette                             Palette
	cfg                                 config.Config
	path                                string
	page, cursor, width, height, offset int
	state                               service.Status
	foreign                             []string
	notice, detail, search              string
	searching, help                     bool
	editor                              textarea.Model
	editing                             string
	confirm                             string
	pending                             []string
	theme                               string
	busy                                bool
}

func New(c config.Config, path string) Model {
	a := textarea.New()
	a.CharLimit = 4 << 20
	a.ShowLineNumbers = true
	return Model{deleteProfile: -1, cfg: c, path: path, width: 80, height: 26, editor: a, theme: LoadTheme(), palette: Theme(LoadTheme()), notice: "r обновить · Tab разделы · ? справка"}
}
func refresh() tea.Msg {
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	return refreshMsg{state: service.State(ctx), foreign: service.OtherActive(ctx)}
}
func (m Model) Init() tea.Cmd { return refresh }
func operation(args []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if len(args) > 0 && args[0] != "stop" {
			if e := service.RequireCurrentBinary(ctx); e != nil {
				return resultMsg{err: e, text: e.Error()}
			}
		}
		var output operationOutput
		cmd := operationCommand(args)
		cmd.Stdout = io.MultiWriter(os.Stdout, &output)
		cmd.Stderr = io.MultiWriter(os.Stderr, &output)
		return tea.ExecProcess(cmd, func(e error) tea.Msg { return resultMsg{err: e, text: output.String()} })()
	}
}

func task(f func(context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, c := context.WithTimeout(context.Background(), 40*time.Second)
		defer c()
		s, e := f(ctx)
		return resultMsg{text: s, err: e}
	}
}
func (m Model) rows() []string {
	var rows []string
	switch m.page {
	case 1, 3:
		for _, p := range m.cfg.Profiles {
			mark := "○"
			if p.Enabled {
				mark = "●"
			}
			strategy, _ := m.cfg.Strategy(p.Strategy)
			rows = append(rows, fmt.Sprintf("%s %-12s %-18s доменов: %d", mark, profileName(p), strategy.Name, len(p.Domains)))
		}
	case 2:
		for _, s := range m.cfg.Strategies {
			rows = append(rows, fmt.Sprintf("%-18s %s", s.Name, s.Description))
		}
	case 6:
		for _, name := range Themes {
			rows = append(rows, ThemeTitle(name))
		}
	}
	return rows
}
func (m Model) visible() []int {
	var out []int
	for i, row := range m.rows() {
		if strings.Contains(strings.ToLower(row), strings.ToLower(m.search)) {
			out = append(out, i)
		}
	}
	return out
}
func (m *Model) index() int {
	v := m.visible()
	if len(v) == 0 {
		return -1
	}
	if m.cursor >= len(v) {
		m.cursor = len(v) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return v[m.cursor]
}
func (m *Model) edit(kind string, index int) {
	var v any
	if kind == "config" {
		v = m.cfg
	} else if kind == "profile" {
		v = m.cfg.Profiles[index]
	} else {
		v = m.cfg.Strategies[index]
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	m.editor.SetValue(string(b))
	m.editor.SetWidth(max(20, m.contentWidth()))
	m.editor.SetHeight(max(4, m.height-13))
	if m.form != nil {
		m.form.name.Width = m.contentWidth()
		m.form.domains.SetWidth(m.contentWidth())
	}
	m.editor.Focus()
	m.editing = kind
	m.offset = index
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
		m.editor.SetWidth(max(20, m.contentWidth()))
		m.editor.SetHeight(max(4, m.height-13))
		if m.form != nil {
			m.form.name.Width = m.contentWidth()
			m.form.domains.SetWidth(m.contentWidth())
		}
		return m, nil
	case refreshMsg:
		m.state = x.state
		m.foreign = x.foreign
		return m, nil
	case resultMsg:
		m.busy = false
		if x.err != nil {
			m.notice = x.err.Error()
			m.detail = x.text
			m.cursor = 0
		} else {
			m.notice = "Готово"
			m.detail = x.text
			m.cursor = 0
		}
		return m, refresh
	case tea.KeyMsg:
		if m.form != nil {
			return m.updateProfile(msg)
		}
		k := x.String()
		if m.editing != "" {
			if k == "esc" {
				m.editing = ""
				return m, nil
			}
			if k == "ctrl+s" {
				candidate := m.cfg
				candidate.Profiles = append([]config.Profile(nil), m.cfg.Profiles...)
				candidate.Strategies = append([]config.Strategy(nil), m.cfg.Strategies...)
				var e error
				switch m.editing {
				case "config":
					candidate, e = config.Parse([]byte(m.editor.Value()))
				case "profile":
					var p config.Profile
					e = json.Unmarshal([]byte(m.editor.Value()), &p)
					if e == nil {
						candidate.Profiles[m.offset] = p
					}
				case "strategy":
					var s config.Strategy
					e = json.Unmarshal([]byte(m.editor.Value()), &s)
					if e == nil {
						candidate.Strategies[m.offset] = s
					}
				}
				if e == nil {
					e = candidate.Validate()
				}
				if e == nil {
					e = config.Save(m.path, candidate)
				}
				if e != nil {
					m.notice = e.Error()
				} else {
					m.cfg = candidate
					m.editing = ""
					m.notice = "Сохранено. Клавиша a применит настройки к службе."
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
		if m.confirm != "" {
			if k == "enter" && m.deleteProfile >= 0 {
				i := m.deleteProfile
				c := m.cfg
				c.Profiles = append([]config.Profile(nil), m.cfg.Profiles[:i]...)
				c.Profiles = append(c.Profiles, m.cfg.Profiles[i+1:]...)
				if e := config.Save(m.path, c); e != nil {
					m.notice = e.Error()
				} else {
					m.cfg = c
					m.notice = "Сервис удалён из черновика; a — применить."
				}
				m.deleteProfile = -1
				m.confirm = ""
				m.cursor = 0
				return m, nil
			}
			if k == "enter" {
				args := m.pending
				m.confirm = ""
				m.pending = nil
				return m, operation(args)
			}
			if k == "esc" || k == "n" {
				m.confirm = ""
				m.deleteProfile = -1
			}
			return m, nil
		}
		if m.searching {
			switch k {
			case "enter", "esc":
				m.searching = false
			case "backspace":
				r := []rune(m.search)
				if len(r) > 0 {
					m.search = string(r[:len(r)-1])
				}
			default:
				if len(x.Runes) > 0 {
					m.search += string(x.Runes)
				}
			}
			m.cursor = 0
			return m, nil
		}
		switch k {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "tab", "right":
			m.page = (m.page + 1) % len(pages)
			m.cursor = 0
			m.search = ""
			m.detail = ""
		case "shift+tab", "left":
			m.page = (m.page + len(pages) - 1) % len(pages)
			m.cursor = 0
			m.search = ""
			m.detail = ""
		case "j", "down":
			m.cursor++
			if m.detail == "" {
				m.index()
			}
		case "k", "up":
			m.cursor = max(0, m.cursor-1)
		case "/":
			m.searching = true
		case "r":
			m.palette = Theme(m.theme)
			return m, refresh
		case "i":
			exe, e := os.Executable()
			if e != nil {
				m.notice = e.Error()
				return m, nil
			}
			return m, tea.ExecProcess(exec.Command(exe, "setup"), func(e error) tea.Msg {
				return resultMsg{err: e, text: "Мастер закрыт. Состояние службы обновлено."}
			})
		case "s":
			action := "start"
			if m.state.Service == "active" {
				action = "stop"
			}
			m.confirm = "Запустить службу FDPI?"
			if action == "stop" {
				m.confirm = "Остановить FDPI и удалить его сетевые правила?"
			}
			m.pending = []string{action}
			if action == "start" && len(m.foreign) > 0 {
				m.confirm = "Остановить " + strings.Join(m.foreign, ", ") + " и запустить FDPI? При неудаче будет предпринято восстановление прежних служб."
				m.pending = append(m.pending, "--replace")
			}
		case "a":
			m.confirm = "Применить настройки? Работающая служба будет перезапущена."
			m.pending = []string{"admin", "apply", m.path}
		case "b":
			m.confirm = "Вернуть предыдущие системные настройки?"
			m.pending = []string{"admin", "rollback"}
		case "n":
			if m.page == 1 {
				m.openProfile(-1)
			}
		case "delete":
			if m.page == 1 {
				if i := m.index(); i >= 0 {
					m.deleteProfile = i
					m.confirm = "Удалить сервис " + profileName(m.cfg.Profiles[i]) + " из черновика?"
					m.pending = nil
				}
			}
		case "e":
			i := m.index()
			if m.page == 1 && i >= 0 {
				m.openProfile(i)
			} else if m.page == 2 && i >= 0 {
				m.edit("strategy", i)
			} else if m.page == 6 {
				m.edit("config", 0)
			}
		case "c":
			if m.page == 2 {
				if i := m.index(); i >= 0 {
					s := m.cfg.Strategies[i]
					s.ID = fmt.Sprintf("custom-%d", time.Now().Unix())
					s.Name += " (копия)"
					candidate := m.cfg
					candidate.Strategies = append(append([]config.Strategy(nil), m.cfg.Strategies...), s)
					if e := config.Save(m.path, candidate); e != nil {
						m.notice = e.Error()
					} else {
						m.cfg = candidate
						m.search = ""
						m.cursor = len(m.cfg.Strategies) - 1
						m.edit("strategy", m.cursor)
					}
				}
			}
		case " ":
			if m.page == 1 {
				if i := m.index(); i >= 0 {
					m.cfg.Profiles[i].Enabled = !m.cfg.Profiles[i].Enabled
					if e := config.Save(m.path, m.cfg); e != nil {
						m.cfg.Profiles[i].Enabled = !m.cfg.Profiles[i].Enabled
						m.notice = e.Error()
					} else {
						m.notice = "Сохранено локально; a — применить к системе"
					}
				}
			}
		case "enter":
			i := m.index()
			if m.page == 6 && i >= 0 {
				m.theme = Themes[i]
				m.palette = Theme(m.theme)
				if e := SaveTheme(m.theme); e != nil {
					m.notice = e.Error()
				}
			} else if m.page == 3 && i >= 0 {
				m.confirm = "Начать подбор? Служба будет перезапускаться. Затем настройки восстановятся."
				m.pending = []string{"lab", m.cfg.Profiles[i].ID}
			} else if m.page == 1 && i >= 0 {
				m.openProfile(i)
			} else if m.page == 2 && i >= 0 {
				m.detail = service.JSON(m.cfg.Strategies[i])
				m.cursor = 0
			}
		case "d":
			if !m.busy {
				m.busy = true
				return m, task(func(ctx context.Context) (string, error) { return diagnostics.ChecksText(diagnostics.Doctor(ctx)), nil })
			}
		case "t":
			if m.page == 1 {
				if i := m.index(); i >= 0 && len(m.cfg.Profiles[i].Domains) > 0 && !m.busy {
					domain := m.cfg.Profiles[i].Domains[0]
					youtube := m.cfg.Profiles[i].ID == "youtube"
					m.busy = true
					return m, task(func(ctx context.Context) (string, error) {
						if youtube {
							return diagnostics.YouTubeText(diagnostics.YouTube(ctx, "")), nil
						}
						return diagnostics.ResultText(diagnostics.Probe(ctx, domain)), nil
					})
				}
			}
		case "l":
			m.page = 5
			m.detail = ""
			return m, task(recentLogs)
		case "esc":
			m.detail = ""
			m.search = ""
			m.help = false
		}
		return m, nil
	}
	return m, nil
}
func (m Model) contentView() string {
	if m.width < 28 || m.height < 10 {
		return "FDPI · увеличьте окно хотя бы до 28×10\nq — выход"
	}
	p := m.palette
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)).Bold(true)
	w := max(20, m.width-6)
	var b strings.Builder

	if m.form != nil {
		b.WriteString(m.profileView())
	} else if m.editing != "" {
		b.WriteString("Редактор: " + editorTitle(m.editing) + " · Ctrl+S сохранить · Esc отмена\n" + m.editor.View())
	} else if m.confirm != "" {
		b.WriteString(accent.Render("ПОДТВЕРЖДЕНИЕ\n") + m.confirm + "\n\nEnter — продолжить · Esc — отмена")
	} else if m.help {
		b.WriteString("j/k ↑/↓ выбор · Tab разделы · / поиск\nПробел — включить сервис · Enter/e — форма сервиса · n — добавить\nc копия стратегии · a применить · b откатить\ns пуск/стоп · t проверка HTTPS · d диагностика\nl журнал · r обновить · Esc назад · q выход\nПодбор: выберите сервис и нажмите Enter.\nНастройки: Enter — тема, e — весь конфиг.\nИзменения применяются к сети клавишей a.\ni — открыть мастер установки по шагам.")
	} else if m.detail != "" {
		lines := strings.Split(m.detail, "\n")
		if m.search != "" {
			var filtered []string
			for _, l := range lines {
				if strings.Contains(strings.ToLower(l), strings.ToLower(m.search)) {
					filtered = append(filtered, l)
				}
			}
			lines = filtered
		}
		start := min(m.cursor, max(0, len(lines)-1))
		end := min(len(lines), start+max(1, m.height-13))
		b.WriteString(strings.Join(lines[start:end], "\n"))
	} else {
		switch m.page {
		case 0:
			b.WriteString(m.dashboard())
		case 4:
			b.WriteString("inet fdpi · исходящий трафик · IPv4 + IPv6\n" + engineLabel() + " · без движка трафик идёт напрямую\nTCP 80/443: первые 20 пакетов\nUDP 443: первые 12 пакетов при включённом QUIC\nДомен из Host/SNI проверяет nfqws2.\n\nДоменные профили не разделяют приложения.\nd — диагностика без изменения сети")
		case 5:
			b.WriteString("l загрузить журнал · / поиск · j/k прокрутка\nДля чтения журнала могут потребоваться права.")
		default:
			rows := m.rows()
			v := m.visible()
			start := max(0, m.cursor-max(1, m.height-12)+1)
			for n := start; n < len(v) && n < start+max(1, m.height-12); n++ {
				row := ansi.Truncate(rows[v[n]], w-3, "…")
				if n == m.cursor {
					b.WriteString(accent.Render("▸ " + row))
				} else {
					b.WriteString("  " + row)
				}
				b.WriteByte('\n')
			}
			if m.page == 1 {
				b.WriteString("\nПробел вкл/выкл · n добавить · Enter изменить\nDel удалить · t тест · a применить")
			}
			if m.page == 2 {
				b.WriteString("\ne изменить · c копия · a применить")
			}
			if m.page == 3 {
				b.WriteString("\nEnter — подобрать; исходные настройки восстановятся")
			}
		}
	}
	if m.searching || m.search != "" {
		b.WriteString("\nПоиск: " + m.search)
	}
	notice := m.notice
	if m.form != nil && m.form.err != "" {
		notice = m.form.err
	}
	if m.busy {
		notice = "Выполняется…"
	}
	b.WriteString("\n\n" + ansi.Truncate(notice, w, "…"))
	lines := strings.Split(ansi.Wrap(b.String(), w, ""), "\n")
	if len(lines) > m.height-2 {
		lines = append(lines[:m.height-3], ansi.Truncate(notice, w, "…"))
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	return strings.Join(lines, "\n")
}
func (m Model) View() string {
	status := "НЕАКТИВЕН"
	if m.state.Service == "active" {
		status = "РАБОТАЕТ"
	}
	labels := []string{"⌂ Обзор", "◇ Сервисы", "⋈ Стратегии", "△ Подбор стратегии", "◎ Сеть", "≡ Журнал", "⚙ Настройки"}
	inner := m
	if m.width >= 76 {
		inner.width -= 28
	}
	inner.height -= 3
	return Frame(m.width, m.height, m.palette, labels, m.page, pages[m.page], status, inner.contentView(), "[Tab] Раздел  [↑↓] Выбор  [S] Старт/Стоп  [R] Обновить  [Q] Выход")
}
func Run(c config.Config, path string) error {
	_, e := tea.NewProgram(New(c, path), tea.WithAltScreen()).Run()
	return e
}
