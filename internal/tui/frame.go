package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const Cyan = "#64d9f2"
const Violet = "#ad91ff"
const Green = "#79ee9a"
const Red = "#ff718b"

// Frame is shared by the application, forms, confirmations and setup wizard.
func Frame(width, height int, p Palette, labels []string, selected int, title, status, content, footer string) string {
	if width < 28 || height < 10 {
		return "FDPI · увеличьте окно до 28×10\nq — выход"
	}
	width = max(26, width-2)
	height = max(8, height-2)
	base := lipgloss.NewStyle().Background(lipgloss.Color(p.Background)).Foreground(lipgloss.Color(p.Foreground))
	border := base.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.Accent))
	fit := func(text string, w, h int) string {
		lines := strings.Split(text, "\n")
		if len(lines) > h {
			lines = lines[:h]
		}
		for len(lines) < h {
			lines = append(lines, "")
		}
		for i := range lines {
			lines[i] = base.Width(w).Render(ansi.Truncate(lines[i], w, "…"))
		}
		return strings.Join(lines, "\n")
	}
	brand := lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan)).Bold(true).Render("FDPI")
	stateColor := p.Foreground
	if status == "РАБОТАЕТ" {
		stateColor = Green
	}
	state := lipgloss.NewStyle().Foreground(lipgloss.Color(stateColor)).Bold(true).Render("● " + status)
	left := brand + "  │  Fast DPI Bypass"
	gap := max(1, width-6-ansi.StringWidth(left)-ansi.StringWidth(state))
	header := border.Padding(0, 1).Width(width - 2).Render(ansi.Truncate(left+strings.Repeat(" ", gap)+state, width-4, "…"))
	bodyHeight := height - 6
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)).Bold(true)
	body := titleStyle.Render(title) + "\n\n" + content
	var center string
	if width >= 76 {
		sideWidth := 23
		mainWidth := width - sideWidth - 5
		var nav []string
		for i, label := range labels {
			line := fmt.Sprintf("  %s", label)
			if i == selected {
				line = lipgloss.NewStyle().Background(lipgloss.Color("#423766")).Foreground(lipgloss.Color("#ffffff")).Bold(true).Width(sideWidth).Render("▎ " + label)
			}
			nav = append(nav, line, "")
		}
		sidebar := border.Width(sideWidth).Render(fit(strings.Join(nav, "\n"), sideWidth, bodyHeight))
		main := border.Padding(0, 1).Width(mainWidth).Render(fit(body, mainWidth-2, bodyHeight))
		center = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
	} else {
		center = border.Padding(0, 1).Width(width - 2).Render(fit(body, width-4, bodyHeight))
	}
	bottom := border.Padding(0, 1).Width(width - 2).Render(ansi.Truncate(footer, width-4, "…"))
	return base.Render(lipgloss.JoinVertical(lipgloss.Left, header, center, bottom))
}

func (m Model) dashboard() string {
	value := func(s string) string { return lipgloss.NewStyle().Foreground(lipgloss.Color(Cyan)).Render(s) }
	status := m.state.StateText()
	if m.state.Service == "active" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color(Green)).Bold(true).Render("Активен")
	}
	auto := m.state.UnitFileState
	if auto == "enabled" {
		auto = "Включён"
	}
	if auto == "" {
		auto = "Не проверен"
	}
	summary := "По профилям"
	seen := map[string]bool{}
	for _, p := range m.cfg.Profiles {
		if p.Enabled {
			seen[p.Strategy] = true
			summary = p.Strategy
		}
	}
	if len(seen) != 1 {
		summary = "По профилям"
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(m.palette.Accent)).Padding(0, 1)
	w := max(22, m.width-10)
	cards := box.Width(w - 2).Render("Статус: " + status + " · " + value("nfqws2") + "\nСтратегия: " + value(summary) + "\nАвтозапуск: " + value(auto))
	if m.width >= 92 {
		cards = lipgloss.JoinHorizontal(lipgloss.Top, box.Width(w/2-3).Render("Статус: "+status+"\nСтратегия: "+value(summary)), box.Width(w/2-3).Render("Движок: "+value("nfqws2")+"\nАвтозапуск: "+value(auto)))
	}
	var rows strings.Builder
	rows.WriteString("Сервисы · локальный черновик\n\n")
	for _, p := range m.cfg.Profiles {
		state := "ВЫКЛ"
		if p.Enabled {
			state = "ВКЛ"
		}
		fmt.Fprintf(&rows, "▸ %-18s %s  %s\n", p.Title(), state, p.Strategy)
	}
	rows.WriteString("\nВКЛ — профиль выбран, а не результат теста.\na — применить · t в Сервисах — проверить доступ")
	return cards + "\n\n" + box.Width(w-2).Render(rows.String()) + "\n\nОчередь: " + m.state.QueueText()
}

func (m Model) contentWidth() int {
	if m.width >= 76 {
		return max(16, m.width-36)
	}
	return max(16, m.width-12)
}
