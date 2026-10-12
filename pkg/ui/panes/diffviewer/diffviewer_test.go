package diffviewer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

func TestRenderPreamble_Empty(t *testing.T) {
	common.RegisterSupportedTints()
	m := New(false, &common.Styles{
		Tint:   common.Themes.Current(),
		Colors: common.Colors{},
	})
	if got := m.renderPreamble(""); got != "" {
		t.Fatalf("expected empty string for empty preamble, got %q", got)
	}
	if got := m.renderPreamble("   \n  \n  "); got != "" {
		t.Fatalf("expected empty string for whitespace-only preamble, got %q", got)
	}
}

func TestRenderPreamble_GitShow(t *testing.T) {
	common.RegisterSupportedTints()
	m := New(false, &common.Styles{
		Tint:   common.Themes.Current(),
		Colors: common.Colors{},
	})
	preamble := `commit abc123def456
Author: Jane Doe <jane@example.com>
Date:   Mon Jan 1 00:00:00 2026 +0000

    feat: add new feature

    This is the body of the commit message.`

	got := m.renderPreamble(preamble)
	plain := ansi.Strip(got)

	// All original content lines should be preserved in the output.
	for _, want := range []string{
		"commit abc123def456",
		"Author: Jane Doe <jane@example.com>",
		"Date:   Mon Jan 1 00:00:00 2026 +0000",
		"feat: add new feature",
		"This is the body of the commit message.",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, plain)
		}
	}
}

func TestRenderPreamble_MergeCommit(t *testing.T) {
	preamble := `commit abc123def456
Merge: aaa111 bbb222
Author: Jane Doe <jane@example.com>
Date:   Mon Jan 1 00:00:00 2026 +0000

    Merge branch 'feature' into main`

	common.RegisterSupportedTints()
	m := New(false, &common.Styles{
		Tint:   common.Themes.Current(),
		Colors: common.Colors{},
	})
	got := m.renderPreamble(preamble)
	plain := ansi.Strip(got)

	for _, want := range []string{
		"Merge: aaa111 bbb222",
		"Merge branch 'feature' into main",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, plain)
		}
	}
}

func TestSideColumn(t *testing.T) {
	lines := []string{"name", "+1 -1", "─────", "Search…", "a", "bb", "c", "40%"}
	got := sideColumn(lines, 5, 4, "┃\n│\n│", 2, "══")
	want := []string{
		"name   ",   // header: padded, blank column
		"+1 -1  ",   //
		"─────══",   // rule row carries the rule to the edge
		"Search…  ", // search line: no track
		"a     ┃",   // diff lines: gap + scrollbar
		"bb    │",
		"c     │",
		"40%    ", // footer: no track
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
	if blank := sideColumn([]string{"x"}, 2, 0, "", -1, "══"); blank[0] != "x   " {
		t.Errorf("no scrollbar: got %q, want blanks", blank[0])
	}
}
