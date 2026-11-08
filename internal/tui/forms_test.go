package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"path/filepath"
	"testing"
)

func TestCustomServiceFormNormalizesAndSaves(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	m := New(config.Default(), p)
	m.page = 1
	m.openProfile(-1)
	m.form.name.SetValue("Мой сервис")
	m.form.domains.SetValue("https://Example.org/watch?a=1\nexample.org, *.cdn.example.net")
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = v.(Model)
	if m.form != nil {
		t.Fatal(m.form.err)
	}
	saved, e := config.Load(p)
	if e != nil {
		t.Fatal(e)
	}
	custom := saved.Profiles[len(saved.Profiles)-1]
	if custom.Name != "Мой сервис" || len(custom.Domains) != 2 || custom.Domains[0] != "example.org" || custom.Domains[1] != "cdn.example.net" {
		t.Fatal(custom)
	}
}
func TestFormRejectsCredentialURLsAndBadDomains(t *testing.T) {
	for _, value := range []string{"https://user:secret@example.com", "ftp://example.com", "not-a-domain", ""} {
		if _, e := parseDomains(value); e == nil {
			t.Fatal("accepted", value)
		}
	}
}
func TestProfileFormArrowStrategyAndSpaceToggle(t *testing.T) {
	m := New(config.Default(), filepath.Join(t.TempDir(), "config.json"))
	m.openProfile(-1)
	m.form.focus = 2
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = v.(Model)
	if m.form.strategy != 1 {
		t.Fatal("arrow did not choose strategy")
	}
	m.form.focus = 3
	before := m.form.enabled
	v, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = v.(Model)
	if m.form.enabled == before {
		t.Fatal("space did not toggle")
	}
}
