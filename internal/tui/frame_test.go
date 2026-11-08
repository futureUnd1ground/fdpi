package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"strings"
	"testing"
)

func TestAllPagesFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{40, 15}, {80, 25}, {136, 38}} {
		m := New(config.Default(), "unused")
		m.width = size[0]
		m.height = size[1]
		for page := range pages {
			m.page = page
			checkFrame(t, m.View(), size)
		}
		m.openProfile(0)
		checkFrame(t, m.View(), size)
		m.form = nil
		m.confirm = "Подтвердите действие"
		checkFrame(t, m.View(), size)
	}
}
func checkFrame(t *testing.T, view string, size [2]int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > size[1] {
		t.Fatalf("height %d exceeds %d", len(lines), size[1])
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > size[0] {
			t.Fatalf("width %d exceeds %d", ansi.StringWidth(line), size[0])
		}
	}
}

func TestUninstallRequiresConfirmation(t *testing.T) {
	m := New(config.Default(), "unused")
	m.page = 6
	m.cursor = len(Themes)
	v, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = v.(Model)
	if cmd != nil || m.confirm == "" || len(m.pending) != 2 || m.pending[0] != "uninstall" {
		t.Fatal("missing uninstall confirmation")
	}
	v, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = v.(Model)
	if cmd != nil || m.confirm != "" {
		t.Fatal("cancel failed")
	}
}
