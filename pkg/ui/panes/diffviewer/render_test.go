package diffviewer

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

// fakeDelta stands in for delta: every input line comes back tagged with
// the width and the theme mode it was rendered for ("[w=80 dark] …"), so a
// test can tell which render is on screen. It counts its runs.
type fakeDelta struct {
	mu   sync.Mutex
	runs int
}

func useFakeDelta(t *testing.T) *fakeDelta {
	t.Helper()
	f := &fakeDelta{}
	prev := runDelta
	runDelta = func(job deltaJob) (string, error) {
		f.mu.Lock()
		f.runs++
		f.mu.Unlock()
		return fakeRender(job), nil
	}
	t.Cleanup(func() { runDelta = prev })
	pinDeltaFeatures(t, true)
	return f
}

func fakeRender(job deltaJob) string {
	width, mode := "?", "dark"
	for _, a := range job.args {
		if w, ok := strings.CutPrefix(a, "-w="); ok {
			width = w
		}
		if a == "--light" || a == "--features=miasma-light" {
			mode = "light"
		}
	}
	var b strings.Builder
	for l := range strings.SplitSeq(strings.TrimRight(job.input, "\n"), "\n") {
		fmt.Fprintf(&b, "[w=%s %s] %s\n", width, mode, l)
	}
	return b.String()
}

// testFiles parses a diff of n files named f0.txt…, each changing lines
// lines.
func testFiles(t *testing.T, n, lines int) []*gitdiff.File {
	t.Helper()
	var b strings.Builder
	for i := range n {
		name := fmt.Sprintf("f%d.txt", i)
		fmt.Fprintf(&b, "diff --git a/%[1]s b/%[1]s\n--- a/%[1]s\n+++ b/%[1]s\n@@ -1,%[2]d +1,%[2]d @@\n", name, lines)
		for j := range lines {
			fmt.Fprintf(&b, "-old %s %d\n+new %s %d\n", name, j, name, j)
		}
	}
	files, _, err := gitdiff.Parse(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != n {
		t.Fatalf("parsed %d files, want %d", len(files), n)
	}
	return files
}

func newRenderTestModel(t *testing.T, theme string, w, h int) Model {
	t.Helper()
	common.RegisterSupportedTints()
	common.Themes.SetTintID(theme)
	t.Cleanup(func() { common.Themes.SetTintID("tokyo_night") })
	styles := common.MakeStyles()
	m := New(false, &styles)
	m.SetSize(w, h)
	return m
}

// runCmd runs cmd and returns its message (nil for none). Batches are not
// expected from the viewer.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

func land(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if msg := runCmd(t, cmd); msg != nil {
		m, _ = m.Update(msg)
	}
	return m
}

func screen(m Model) string {
	return ansi.Strip(m.View())
}

// gd then j straight away: the directory render lands after the file's and
// must not replace it.
func TestLateDirRenderDoesNotReplaceFile(t *testing.T) {
	useFakeDelta(t)
	files := testFiles(t, 3, 2)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)

	m, dirCmd := m.SetDirPatch("/", files)
	m, fileCmd := m.SetFilePatch(files[1])
	m = land(t, m, fileCmd)
	m = land(t, m, dirCmd) // late

	s := screen(m)
	if !strings.Contains(s, "new f1.txt 0") {
		t.Fatalf("the file's render should be on screen:\n%s", s)
	}
	if strings.Contains(s, "f0.txt") {
		t.Fatalf("the late directory render replaced the file view:\n%s", s)
	}
	if m.shown == nil || m.shown.isDir {
		t.Fatalf("shown = %+v, want the file render", m.shown)
	}
}

// A dark render landing after the light one (a late terminal reply, a
// theme preview) must not stay on screen under the light chrome.
func TestLateThemeRenderIsDropped(t *testing.T) {
	useFakeDelta(t)
	files := testFiles(t, 2, 2)
	m := newRenderTestModel(t, "miasma_dark", 100, 30)

	m, darkCmd := m.SetDirPatch("/", files)
	common.Themes.SetTintID("miasma_light")
	styles := common.MakeStyles()
	lightCmd := m.UpdateTheme(&styles)
	m = land(t, m, lightCmd)
	m = land(t, m, darkCmd)

	if s := screen(m); strings.Contains(s, " dark]") || !strings.Contains(s, " light]") {
		t.Fatalf("the dark render replaced the light one:\n%s", s)
	}
}

// A render already done is shown again from the cache, without delta.
func TestRenderReusedFromCache(t *testing.T) {
	f := useFakeDelta(t)
	files := testFiles(t, 3, 2)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)

	m, cmd := m.SetDirPatch("/", files)
	m = land(t, m, cmd)
	m, cmd = m.SetFilePatch(files[0])
	m = land(t, m, cmd)
	runs := f.runs
	m, cmd = m.SetDirPatch("/", files)
	if cmd != nil {
		t.Fatal("a cached directory render should show without a new render")
	}
	if f.runs != runs || !strings.Contains(screen(m), "f2.txt") {
		t.Fatalf("expected the cached directory render on screen (runs %d → %d):\n%s", runs, f.runs, screen(m))
	}
}

// After a re-render the last jump no longer applies: its index belongs to
// the old render's file offsets.
func TestRerenderForgetsJump(t *testing.T) {
	useFakeDelta(t)
	files := testFiles(t, 3, 2)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)
	m, cmd := m.SetDirPatch("/", files)
	m = land(t, m, cmd)
	if !m.JumpFile(1) || !m.jump.ok {
		t.Fatal("precondition: the jump should be remembered")
	}
	m = land(t, m, m.SetSideBySide(true))
	if m.jump.ok {
		t.Fatalf("the jump survived a re-render: %+v", m.jump)
	}
}

// The delta args are built when the render is asked for, on the main loop:
// a theme change after that does not leak into the running render.
func TestRenderArgsCapturedAtRequest(t *testing.T) {
	useFakeDelta(t)
	files := testFiles(t, 1, 1)
	m := newRenderTestModel(t, "miasma_dark", 100, 30)
	m, cmd := m.SetFilePatch(files[0])
	common.Themes.SetTintID("miasma_light") // as the picker's preview would
	msg := runCmd(t, cmd).(diffContentMsg)
	if got := ansi.Strip(msg.lines[0].item.Content()); !strings.Contains(got, " dark]") {
		t.Fatalf("the render read the theme after it was asked for: %q", got)
	}
}

func TestRenderFilesKeepsOrder(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("f%02d", i)
	}
	outs, err := renderFiles(len(names), func(i int) (string, error) {
		return names[i] + "\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, out := range outs {
		if want := names[i] + "\n"; out != want {
			t.Fatalf("outs[%d] = %q, want %q", i, out, want)
		}
	}
	_, err = renderFiles(len(names), func(i int) (string, error) {
		if i == 7 {
			return "", fmt.Errorf("boom")
		}
		return "", nil
	})
	if err == nil {
		t.Fatal("expected the render error")
	}
}
