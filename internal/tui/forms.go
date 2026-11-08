package tui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type profileForm struct {
	name                   textinput.Model
	domains                textarea.Model
	focus, strategy, index int
	enabled                bool
	id, err                string
}

func profileName(p config.Profile) string { return p.Title() }
func (m *Model) openProfile(index int) {
	if len(m.cfg.Strategies) == 0 {
		m.notice = "Сначала добавьте стратегию."
		return
	}
	n := textinput.New()
	n.CharLimit = 60
	n.Placeholder = "Например: Мой сайт"
	n.Width = m.contentWidth()
	d := textarea.New()
	d.CharLimit = 100000
	d.SetHeight(max(2, min(4, m.height-24)))
	d.SetWidth(m.contentWidth())
	d.Placeholder = "example.org\nhttps://video.example.net/"
	d.ShowLineNumbers = false
	f := profileForm{name: n, domains: d, index: index, enabled: true, id: "custom-" + strconv.FormatInt(time.Now().UnixNano(), 36)}
	if index >= 0 {
		p := m.cfg.Profiles[index]
		f.id = p.ID
		f.name.SetValue(profileName(p))
		f.domains.SetValue(strings.Join(p.Domains, "\n"))
		f.enabled = p.Enabled
		for i, s := range m.cfg.Strategies {
			if s.ID == p.Strategy {
				f.strategy = i
			}
		}
	}
	f.name.Focus()
	m.form = &f
	m.detail = ""
}
func parseDomains(text string) ([]string, error) {
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' ' || r == '\t' })
	seen := map[string]bool{}
	var domains []string
	for _, raw := range parts {
		d := strings.ToLower(strings.TrimSpace(raw))
		if strings.Contains(d, "://") {
			u, e := url.Parse(d)
			if e != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
				return nil, fmt.Errorf("нужен домен или адрес http/https")
			}
			d = u.Hostname()
		}
		d = strings.TrimPrefix(d, "*.")
		d = strings.TrimSuffix(d, ".")
		if !config.Domain(d) {
			return nil, fmt.Errorf("некорректный домен: %s", d)
		}
		if !seen[d] {
			domains = append(domains, d)
			seen[d] = true
		}
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("добавьте хотя бы один домен")
	}
	return domains, nil
}
func (m Model) updateProfile(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.form
	save := false
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch k {
		case "esc":
			m.form = nil
			return m, nil
		case "ctrl+s":
			save = true
		case "tab":
			f.focus = (f.focus + 1) % 6
		case "shift+tab":
			f.focus = (f.focus + 5) % 6
		case "up":
			if f.focus != 1 {
				f.focus = max(0, f.focus-1)
			} else {
				var cmd tea.Cmd
				f.domains, cmd = f.domains.Update(msg)
				return m, cmd
			}
		case "down":
			if f.focus != 1 {
				f.focus = min(5, f.focus+1)
			} else {
				var cmd tea.Cmd
				f.domains, cmd = f.domains.Update(msg)
				return m, cmd
			}
		case "left", "right":
			if f.focus == 2 {
				delta := 1
				if k == "left" {
					delta = -1
				}
				f.strategy = (f.strategy + delta + len(m.cfg.Strategies)) % len(m.cfg.Strategies)
			} else {
				if f.focus == 0 {
					var cmd tea.Cmd
					f.name, cmd = f.name.Update(msg)
					return m, cmd
				}
				if f.focus == 1 {
					var cmd tea.Cmd
					f.domains, cmd = f.domains.Update(msg)
					return m, cmd
				}
			}
		case " ":
			if f.focus == 3 {
				f.enabled = !f.enabled
			} else if f.focus == 0 {
				var cmd tea.Cmd
				f.name, cmd = f.name.Update(msg)
				return m, cmd
			} else if f.focus == 1 {
				var cmd tea.Cmd
				f.domains, cmd = f.domains.Update(msg)
				return m, cmd
			}
		case "enter":
			switch f.focus {
			case 0:
				f.focus = 1
			case 1:
				var cmd tea.Cmd
				f.domains, cmd = f.domains.Update(msg)
				return m, cmd
			case 2:
				f.strategy = (f.strategy + 1) % len(m.cfg.Strategies)
			case 3:
				f.enabled = !f.enabled
			case 4:
				save = true
			case 5:
				m.form = nil
				return m, nil
			}
		default:
			if f.focus == 0 {
				var cmd tea.Cmd
				f.name, cmd = f.name.Update(msg)
				return m, cmd
			}
			if f.focus == 1 {
				var cmd tea.Cmd
				f.domains, cmd = f.domains.Update(msg)
				return m, cmd
			}
		}
	}
	f.name.Blur()
	f.domains.Blur()
	if f.focus == 0 {
		f.name.Focus()
	}
	if f.focus == 1 {
		f.domains.Focus()
	}
	if save {
		domains, e := parseDomains(f.domains.Value())
		if e != nil {
			f.err = e.Error()
			return m, nil
		}
		if strings.TrimSpace(f.name.Value()) == "" {
			f.err = "Введите название сервиса"
			return m, nil
		}
		p := config.Profile{ID: f.id, Name: strings.TrimSpace(f.name.Value()), Domains: domains, Strategy: m.cfg.Strategies[f.strategy].ID, Enabled: f.enabled}
		c := m.cfg
		c.Profiles = append([]config.Profile(nil), m.cfg.Profiles...)
		if f.index < 0 {
			c.Profiles = append(c.Profiles, p)
		} else {
			c.Profiles[f.index] = p
		}
		if e = config.Save(m.path, c); e != nil {
			f.err = e.Error()
			return m, nil
		}
		m.cfg = c
		m.form = nil
		m.search = ""
		m.notice = "Сервис сохранён. a — применить настройки к сети."
		m.cursor = max(0, f.index)
		if f.index < 0 {
			m.cursor = len(c.Profiles) - 1
		}
	}
	return m, nil
}
func (m Model) profileView() string {
	f := m.form
	title := "Новый сервис"
	if f.index >= 0 {
		title = "Изменить сервис"
	}
	marker := func(i int) string {
		if f.focus == i {
			return "▸ "
		}
		return "  "
	}
	enabled := "[ ] Выключен"
	if f.enabled {
		enabled = "[✓] Включён"
	}
	return title + "\nTab — поле · Ctrl+S — сохранить · Esc — отменить\n\n" + marker(0) + "Название: " + f.name.View() + "\n" + marker(1) + "Домены или ссылки\n" + f.domains.View() + "\n" + marker(2) + "Стратегия: " + m.cfg.Strategies[f.strategy].Name + "\n" + marker(3) + enabled + "\n" + marker(4) + "[ Сохранить ]   " + marker(5) + "[ Отмена ]\n" + f.err
}
