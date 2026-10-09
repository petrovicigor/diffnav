package filetree

import (
	"os"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/dlvhdr/diffnav/pkg/config"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

func TestRevealLine(t *testing.T) {
	cases := []struct {
		name                         string
		line, top, height, off, want int
	}{
		{"visible stays put", 10, 5, 20, 3, 5},
		{"above scrolls up", 2, 5, 20, 3, 0},
		{"near top edge keeps scrolloff", 7, 5, 20, 3, 4},
		{"below scrolls down", 40, 5, 20, 3, 24},
		{"near bottom edge keeps scrolloff", 23, 5, 20, 3, 7},
		{"tiny viewport clamps scrolloff", 10, 0, 3, 3, 9},
		{"no height", 10, 4, 0, 3, 4},
	}
	for _, c := range cases {
		if got := revealLine(c.line, c.top, c.height, c.off); got != c.want {
			t.Errorf("%s: revealLine(%d, %d, %d, %d) = %d, want %d",
				c.name, c.line, c.top, c.height, c.off, got, c.want)
		}
	}
}

func TestSetCurrentFileMarksOnlyThatFile(t *testing.T) {
	f, err := os.Open("testdata/multiple_files.diff")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := gitdiff.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	common.RegisterSupportedTints()
	common.Themes.SetTintID("miasma_dark")
	styles := common.MakeStyles()

	m := New(config.DefaultConfig(), &styles)
	m.SetSize(40, 20)
	m = m.SetFiles(files)

	currentBg := "48;2;47;52;36" // Miasma dark CurrentBg #2f3424
	if strings.Contains(m.View(), currentBg) {
		t.Fatal("no file should be marked before SetCurrentFile")
	}

	copied := m // the tui copies the model around; the mark must follow
	if !copied.SetCurrentFile("yarn.lock") {
		t.Fatal("SetCurrentFile reported no change")
	}
	if copied.SetCurrentFile("yarn.lock") {
		t.Fatal("setting the same file again should report no change")
	}
	marked := []string{}
	for _, line := range strings.Split(copied.View(), "\n") {
		if strings.Contains(line, currentBg) {
			marked = append(marked, ansi.Strip(line))
		}
	}
	if len(marked) != 1 || !strings.Contains(marked[0], "yarn.lock") {
		t.Fatalf("expected only yarn.lock marked, got %q", marked)
	}
	if m.CurrentFile() != "yarn.lock" {
		t.Fatalf("the original copy should share the mark, got %q", m.CurrentFile())
	}

	copied.SetCurrentFile("")
	if strings.Contains(copied.View(), currentBg) {
		t.Fatal("clearing the mark should remove the highlight")
	}
}
