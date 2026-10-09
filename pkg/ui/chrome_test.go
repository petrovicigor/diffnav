package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDefaultChromeHasNoTitleOrHelpHint(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "DIFFNAV") {
		t.Error("the DIFFNAV title should be hidden by default")
	}
	if strings.Contains(view, "F1/? help") {
		t.Error("the help hint should be hidden by default")
	}
	if !strings.Contains(view, "files") {
		t.Error("the footer stats should stay")
	}
	if got := len(strings.Split(m.View().Content, "\n")); got != 30 {
		t.Errorf("view is %d rows, want the window's 30", got)
	}
}

func TestHeaderShowsWithCommitInfoOnly(t *testing.T) {
	m := newTestMainModel(t)
	if m.showHeader() {
		t.Fatal("no title and no commit info: the header should be hidden")
	}
	m.cachedMeta = commitMeta{hash: "abc1234", author: "JDoe"}
	if !m.showHeader() {
		t.Fatal("commit info should bring the header back")
	}
	m.width = 80
	if h := ansi.Strip(m.viewHeader()); !strings.HasPrefix(h, " abc1234") {
		t.Fatalf("header without title should lead with the hash, got %q", h)
	}
	m.config.UI.HideHeader = true
	if m.showHeader() {
		t.Fatal("hideHeader still wins")
	}
}

func TestOptInChromeStillWorks(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideTitle = false
	m.config.UI.HideHelpHint = false
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "DIFFNAV") || !strings.Contains(view, "F1/? help") {
		t.Fatal("hideTitle/hideHelpHint: false should bring the title and hint back")
	}
	if got := len(strings.Split(m.View().Content, "\n")); got != 30 {
		t.Errorf("view is %d rows, want the window's 30", got)
	}
}
