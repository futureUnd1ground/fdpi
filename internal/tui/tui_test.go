package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResponsiveAndEditor(t *testing.T) {
	m := New(config.Default(), filepath.Join(t.TempDir(), "config.json"))
	for _, size := range [][2]int{{80, 25}, {40, 15}, {120, 40}, {20, 8}} {
		v, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = v.(Model)
		if !strings.Contains(m.View(), "FDPI") {
			t.Fatal("missing title")
		}
	}
	m.page = 1
	m.width = 80
	m.height = 25
	m.edit("profile", 0)
	if m.editing != "profile" {
		t.Fatal("editor failed")
	}
	m.editor.SetValue(`{"id":"../escape"}`)
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = v.(Model)
	if m.editing == "" {
		t.Fatal("invalid input saved")
	}
}
func TestPaletteContrast(t *testing.T) {
	p := filepath.Join(t.TempDir(), "colors.json")
	if e := os.WriteFile(p, []byte(`{"special":{"background":"#111111","foreground":"#121212"},"colors":{"color4":"#111112"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	v, e := ReadPalette(p, "test")
	if e != nil {
		t.Fatal(e)
	}
	if contrast(v.Background, v.Foreground) < 4.5 || contrast(v.Background, v.Accent) < 3 {
		t.Fatal(v)
	}
}
func BenchmarkInitialView(b *testing.B) {
	m := New(config.Default(), "unused")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}
