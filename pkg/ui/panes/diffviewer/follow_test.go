package diffviewer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestJoinFileRenders(t *testing.T) {
	text, starts := joinFileRenders("commit abc\n\n", []string{
		"a1\na2\na3\n",
		"b1\n",
		"c1\nc2", // no final newline
	})
	if want := []int{2, 5, 6}; fmt.Sprint(starts) != fmt.Sprint(want) {
		t.Fatalf("starts = %v, want %v", starts, want)
	}
	lines := strings.Split(text, "\n")
	for i, want := range map[int]string{2: "a1", 5: "b1", 6: "c1", 7: "c2"} {
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q (starts must index the joined text)", i, lines[i], want)
		}
	}
	if !strings.HasSuffix(text, "\n") {
		t.Error("joined text should end in a newline")
	}
}

func TestFileIndexAt(t *testing.T) {
	starts := []int{2, 10, 11, 30}
	cases := map[int]int{0: -1, 1: -1, 2: 0, 9: 0, 10: 1, 11: 2, 29: 2, 30: 3, 500: 3}
	for line, want := range cases {
		if got := fileIndexAt(starts, line); got != want {
			t.Errorf("fileIndexAt(%d) = %d, want %d", line, got, want)
		}
	}
	if got := fileIndexAt(nil, 5); got != -1 {
		t.Errorf("no files: got %d, want -1", got)
	}
}

func TestCurrentFileIdx_JumpStaysCurrentUntilScroll(t *testing.T) {
	starts := []int{0, 50, 95} // the last file is too short to reach the top
	j := fileJump{ok: true, key: "/", idx: 2, top: 80}
	if got := currentFileIdx(starts, 80, j, "/"); got != 2 {
		t.Errorf("right after the jump: got %d, want 2", got)
	}
	if got := currentFileIdx(starts, 79, j, "/"); got != 1 {
		t.Errorf("after scrolling: got %d, want 1", got)
	}
	if got := currentFileIdx(starts, 80, j, "/app"); got != 1 {
		t.Errorf("in another render: got %d, want 1", got)
	}
}

func TestJumpTarget(t *testing.T) {
	starts := []int{3, 20, 40}
	cases := []struct {
		name            string
		top, cur, delta int
		want            int
	}{
		{"next from preamble", 0, -1, 1, 0},
		{"next", 3, 0, 1, 1},
		{"next mid-file", 25, 1, 1, 2},
		{"next past last", 45, 2, 1, -1},
		{"prev mid-file returns to its start", 25, 1, -1, 1},
		{"prev at file start", 20, 1, -1, 0},
		{"prev at first file start", 3, 0, -1, -1},
		{"prev from preamble", 0, -1, -1, -1},
	}
	for _, c := range cases {
		if got := jumpTarget(starts, c.top, c.cur, c.delta); got != c.want {
			t.Errorf("%s: jumpTarget(top=%d, cur=%d, %+d) = %d, want %d",
				c.name, c.top, c.cur, c.delta, got, c.want)
		}
	}
}

// followModel renders n files of lines changed lines each ("/" view, with
// preamble when non-empty) into a 100×h viewer through the real viewport.
func followModel(t *testing.T, n, lines, h int, preamble string) Model {
	t.Helper()
	return followModelOf(t, testFiles(t, n, lines), h, preamble)
}

func followModelOf(t *testing.T, files []*gitdiff.File, h int, preamble string) Model {
	t.Helper()
	useFakeDelta(t)
	m := newRenderTestModel(t, "tokyo_night", 100, h)
	m.SetPreamble(preamble)
	m, cmd := m.SetDirPatch("/", files)
	m = land(t, m, cmd)
	if m.followable() == nil {
		t.Fatal("precondition: the directory render should be followable")
	}
	return m
}

func TestFollow_TopLineAndJumps(t *testing.T) {
	m := followModel(t, 5, 6, 20, "")
	starts := m.shown.fileStarts
	if got := m.CurrentFilePath(); got != "f0.txt" {
		t.Fatalf("at the top: %q, want f0.txt", got)
	}
	m.ScrollDown(starts[2] + 3)
	if got := m.CurrentFilePath(); got != "f2.txt" {
		t.Fatalf("scrolled into f2: %q", got)
	}
	if !m.JumpFile(1) || m.topLine() != starts[3] || m.CurrentFilePath() != "f3.txt" {
		t.Fatalf("] should put f3 at the top: top %d (f3 starts %d), current %q", m.topLine(), starts[3], m.CurrentFilePath())
	}
	m.ScrollDown(2)
	if !m.JumpFile(-1) || m.topLine() != starts[3] {
		t.Fatalf("[ inside f3 should return to its start, top %d", m.topLine())
	}
	if !m.JumpFile(-1) || m.topLine() != starts[2] {
		t.Fatalf("[ at f3's start should go to f2, top %d", m.topLine())
	}
}

func TestFollow_PreambleIsNoFile(t *testing.T) {
	m := followModel(t, 3, 6, 20, "commit abc123\nAuthor: x\n\n    msg")
	if got := m.CurrentFilePath(); got != "" {
		t.Fatalf("the preamble is no file, got %q", got)
	}
	if !m.JumpFile(1) || m.CurrentFilePath() != "f0.txt" || m.topLine() != m.shown.fileStarts[0] {
		t.Fatalf("] from the preamble should go to f0, top %d current %q", m.topLine(), m.CurrentFilePath())
	}
}

// The last file is too short to reach the top: a jump to it stays current
// until the viewport moves.
func TestFollow_ShortLastFile(t *testing.T) {
	files := append(testFiles(t, 3, 20), testFiles(t, 4, 1)[3]) // f3.txt: 1 line
	m := followModelOf(t, files, 30, "")
	starts := m.shown.fileStarts
	m.ScrollDown(starts[2])
	if !m.JumpFile(1) || m.CurrentFilePath() != "f3.txt" {
		t.Fatalf("] to the short last file: current %q", m.CurrentFilePath())
	}
	if m.topLine() >= starts[3] {
		t.Fatal("precondition: the last file should be too short to reach the top")
	}
	m.ScrollUp(1)
	if got := m.CurrentFilePath(); got != "f2.txt" {
		t.Fatalf("after scrolling up the top line's file is current, got %q", got)
	}
}

// With selection on, ] puts the next file at the top with its first line
// selected, the tree follows the selected line, and [ inside a file goes to
// that file's start.
func TestFollow_SelectionMode(t *testing.T) {
	m := followModel(t, 5, 6, 20, "")
	starts := m.shown.fileStarts
	m.SetSelectionEnabled(true)
	if !m.JumpFile(1) {
		t.Fatal("] did nothing")
	}
	if m.topLine() != starts[1] || m.fvp.GetSelectedItemIdx() != starts[1] {
		t.Fatalf("] should put f1 at the top, selected: top %d sel %d, f1 starts %d",
			m.topLine(), m.fvp.GetSelectedItemIdx(), starts[1])
	}
	m.fvp.SetSelectedItemIdx(starts[2] + 2) // moved within the view, top unchanged
	if got := m.CurrentFilePath(); got != "f2.txt" {
		t.Fatalf("the tree should follow the selected line, got %q", got)
	}
	if !m.JumpFile(-1) || m.fvp.GetSelectedItemIdx() != starts[2] {
		t.Fatalf("[ should go to the start of the selected file (f2 at %d), selected %d", starts[2], m.fvp.GetSelectedItemIdx())
	}
}
