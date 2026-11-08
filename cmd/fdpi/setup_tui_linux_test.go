package main

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"path/filepath"
	"testing"
)

func TestInstallerUsesArrowsAndCheckboxes(t *testing.T) {
	m := newSetupModel(context.Background(), findSource())
	m.cfg = config.Default()
	m.loaded = true
	m.page = 1
	m.cursor = 0
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = v.(setupModel)
	if !m.cfg.Profiles[0].Enabled {
		t.Fatal("service checkbox failed")
	}
	m.cursor = len(m.cfg.Profiles) + 1
	v, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = v.(setupModel)
	if !m.autostart || !m.persist {
		t.Fatal("autostart requires persistent accounting")
	}
	m.cursor = len(m.cfg.Profiles)
	v, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = v.(setupModel)
	if m.autostart {
		t.Fatal("autostart remained with persistence disabled")
	}
}
func TestInstallerFailureDoesNotAdvance(t *testing.T) {
	m := newSetupModel(context.Background(), findSource())
	m.page = 2
	m.steps = []installStep{{title: "failed"}, {title: "must not run"}}
	m.busy = true
	v, _ := m.Update(installDone{err: context.DeadlineExceeded})
	m = v.(setupModel)
	if m.current != 0 || len(m.completed) != 0 || m.err == "" {
		t.Fatal("failure advanced installer")
	}
}
func TestOnboardingVersionMarker(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if !needsOnboarding() {
		t.Fatal("first run skipped")
	}
	if e := config.Write(onboardingPath(), []byte(`{"version":"`+version+`"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if needsOnboarding() {
		t.Fatal("dismissed onboarding repeated")
	}
	if filepath.Base(onboardingPath()) != "onboarding.json" {
		t.Fatal("wrong marker")
	}
}
