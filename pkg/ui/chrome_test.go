package ui

import (
	"slices"
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

// chromeRows renders m at w×h with the root directory in the diff viewer
// and returns the plain rows.
func chromeRows(t *testing.T, m mainModel, w, h int) []string {
	t.Helper()
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	m.diffViewer, _ = m.diffViewer.SetDirPatch("/", m.files)
	return strings.Split(ansi.Strip(m.View().Content), "\n")
}

// The diff header's rule joins the tree's separator with "├" and runs to
// the right edge, and no row is wider than the window.
func TestChrome_HeaderRuleJoinsSeparator(t *testing.T) {
	for _, tree := range []bool{true, false} {
		m := newTestMainModel(t)
		m.isShowingFileTree = tree
		rows := chromeRows(t, m, 120, 30)
		for i, r := range rows {
			if w := ansi.StringWidth(r); w > 120 {
				t.Errorf("tree=%v row %d is %d wide, window is 120: %q", tree, i, w, r)
			}
		}
		top := []rune(rows[0])
		wantCorner, col := '┬', m.sidebarWidth()
		if !tree {
			wantCorner, col = '┌', 0
		}
		if len(top) <= col || top[col] != wantCorner {
			t.Errorf("tree=%v: top line should have %q at column %d: %q", tree, wantCorner, col, rows[0])
		}
		rule := -1
		for i, r := range rows {
			if strings.Contains(r, "├") {
				rule = i
				break
			}
		}
		if rule < 0 {
			t.Fatalf("tree=%v: no ├ joint in\n%s", tree, strings.Join(rows, "\n"))
		}
		if !strings.Contains(rows[rule+1], "Search…") {
			t.Errorf("tree=%v: the joint should sit on the header rule above the search line, row %d: %q", tree, rule, rows[rule])
		}
		line := []rune(rows[rule])
		joint := slices.Index(line, '├')
		if joint != col {
			t.Errorf("tree=%v: ├ at column %d, separator is at %d", tree, joint, col)
		}
		if got := ansi.StringWidth(string(line[joint:])); got != 120-col {
			t.Errorf("tree=%v: rule should run to the right edge, ends %d short: %q", tree, 120-col-got, rows[rule])
		}
		if strings.Contains(string(line[joint+1:]), "│") {
			t.Errorf("tree=%v: the scrollbar must not cut the header rule: %q", tree, rows[rule])
		}
	}
}
