package diffviewer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	renders.reset()
	t.Cleanup(renders.reset)
	runDelta = func(_ context.Context, job deltaJob) (string, error) {
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
	m.SetStyles(&styles)
	lightCmd := m.UpdateTheme()
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
	outs, err := renderFiles(context.Background(), len(names), func(_ context.Context, i int) (string, error) {
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
	_, err = renderFiles(context.Background(), len(names), func(_ context.Context, i int) (string, error) {
		if i == 7 {
			return "", fmt.Errorf("boom")
		}
		return "", nil
	})
	if err == nil {
		t.Fatal("expected the render error")
	}
}

// A render superseded by a newer request is cancelled: its delta processes
// stop and it sends nothing.
func TestSupersededRenderIsCancelled(t *testing.T) {
	useFakeDelta(t)
	started := make(chan struct{}, 64)
	var block atomic.Bool
	block.Store(true)
	runDelta = func(ctx context.Context, job deltaJob) (string, error) {
		if !block.Load() {
			return fakeRender(job), nil
		}
		started <- struct{}{}
		<-ctx.Done() // a delta that would run forever
		return "", ctx.Err()
	}

	files := testFiles(t, 20, 1)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)
	m, dirCmd := m.SetDirPatch("/", files)
	done := make(chan tea.Msg, 1)
	go func() { done <- dirCmd() }()
	<-started

	block.Store(false) // the file render runs normally
	m, fileCmd := m.SetFilePatch(files[3])
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatalf("a cancelled render sent %T", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the superseded render kept running")
	}
	m = land(t, m, fileCmd)
	if !strings.Contains(screen(m), "new f3.txt 0") {
		t.Fatalf("the file render should be on screen:\n%s", screen(m))
	}
}

// However many renders run, at most maxParallelDeltas deltas run at once.
func TestDeltaProcessesBoundedAcrossRenders(t *testing.T) {
	useFakeDelta(t)
	var mu sync.Mutex
	running, peak := 0, 0
	runDelta = func(_ context.Context, job deltaJob) (string, error) {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		running--
		mu.Unlock()
		return fakeRender(job), nil
	}
	var wg sync.WaitGroup
	for w := range 4 { // four views rendering at once, e.g. a resize storm
		m := newRenderTestModel(t, "tokyo_night", 80+w, 30)
		_, cmd := m.SetDirPatch("/", testFiles(t, 30, 1))
		wg.Add(1)
		go func() { defer wg.Done(); cmd() }()
	}
	wg.Wait()
	if peak > maxParallelDeltas {
		t.Fatalf("%d deltas ran at once, the cap is %d", peak, maxParallelDeltas)
	}
}

// A watch reload that changes one file re-runs delta for that file only.
func TestRenderCachePerFile(t *testing.T) {
	f := useFakeDelta(t)
	files := testFiles(t, 5, 2)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)
	m, cmd := m.SetDirPatch("/", files)
	m = land(t, m, cmd)
	if f.runs != 5 {
		t.Fatalf("first render ran %d deltas, want 5", f.runs)
	}

	changed := testFiles(t, 5, 2)
	changed[2] = testFiles(t, 3, 3)[2] // f2.txt, now 3 lines
	m.ClearCache()
	m, cmd = m.SetDirPatch("/", changed)
	m = land(t, m, cmd)
	if f.runs != 6 {
		t.Fatalf("re-render ran %d deltas, want 1 (only the changed file)", f.runs-5)
	}
	if s := shownText(m); !strings.Contains(s, "new f2.txt 2") || !strings.Contains(s, "f4.txt") {
		t.Fatalf("expected the new render on screen:\n%s", s)
	}
}

// A height change (the header coming or going) does not re-render.
func TestHeightOnlyResizeKeepsRender(t *testing.T) {
	f := useFakeDelta(t)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)
	m, cmd := m.SetDirPatch("/", testFiles(t, 3, 1))
	m = land(t, m, cmd)
	runs := f.runs
	if cmd := m.SetSize(100, 25); cmd != nil {
		t.Fatal("a height-only resize should not render")
	}
	if f.runs != runs {
		t.Fatal("a height-only resize ran delta")
	}
}

// A resize storm renders once, at the final width, after it settles.
func TestResizeStormRendersOnce(t *testing.T) {
	f := useFakeDelta(t)
	prev := resizeDebounce
	resizeDebounce = time.Millisecond
	t.Cleanup(func() { resizeDebounce = prev })

	m := newRenderTestModel(t, "tokyo_night", 100, 30)
	m, cmd := m.SetDirPatch("/", testFiles(t, 4, 1))
	m = land(t, m, cmd)
	runs := f.runs

	var ticks []tea.Cmd
	for w := 101; w <= 120; w++ {
		ticks = append(ticks, m.SetSize(w, 30))
	}
	renders := 0
	for _, tick := range ticks {
		var render tea.Cmd
		m, render = m.Update(runCmd(t, tick))
		if msg := runCmd(t, render); msg != nil {
			if _, ok := msg.(diffContentMsg); ok {
				renders++
				m, _ = m.Update(msg)
			}
		}
	}
	if renders != 1 || f.runs-runs != 4 {
		t.Fatalf("storm: %d renders, %d deltas; want 1 render of 4 files", renders, f.runs-runs)
	}
	if s := screen(m); !strings.Contains(s, "[w=117 ") {
		t.Fatalf("the render should be at the final width (120 - %d):\n%s", scrollbarWidth, s)
	}
	// Back to a width rendered before: from the cache, no wait.
	if cmd := m.SetSize(100, 30); cmd != nil {
		if msg := runCmd(t, cmd); msg != nil {
			t.Fatalf("a cached width should show at once, got %T", msg)
		}
	}
	if !strings.Contains(screen(m), "[w=97 ") {
		t.Fatalf("the cached render at the old width should be back:\n%s", screen(m))
	}
}

// shownText is every line of the render on screen, scrolled or not.
func shownText(m Model) string {
	if m.shown == nil {
		return ""
	}
	lines := make([]string, len(m.shown.diff))
	for i, l := range m.shown.diff {
		lines[i] = l.item.ContentNoAnsi()
	}
	return strings.Join(lines, "\n")
}

// A file delta fails on shows as its plain patch with a note; the other
// files render, and nothing crashes.
func TestFailedFileRendersInline(t *testing.T) {
	useFakeDelta(t)
	runDelta = func(_ context.Context, job deltaJob) (string, error) {
		if strings.Contains(job.input, "f1.txt") {
			return "", fmt.Errorf("exit status 1")
		}
		return fakeRender(job), nil
	}
	files := testFiles(t, 3, 1)
	m := newRenderTestModel(t, "tokyo_night", 100, 30)
	m, cmd := m.SetDirPatch("/", files)
	msg, ok := runCmd(t, cmd).(diffContentMsg)
	if !ok {
		t.Fatalf("one failing file must not fail the view")
	}
	m, _ = m.Update(msg)
	s := shownText(m)
	for _, want := range []string{"[w=97 dark] +new f0.txt 0", "⚠ delta failed on f1.txt: exit status 1", "+new f1.txt 0", "[w=97 dark] +new f2.txt 0"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if got := m.shown.filePaths[fileIndexAt(m.shown.fileStarts, strings.Count(s[:strings.Index(s, "⚠")], "\n"))]; got != "f1.txt" {
		t.Errorf("the note should sit at f1.txt's offset, is in %s", got)
	}

	// No delta at all is still fatal: nothing could render.
	runDelta = func(context.Context, deltaJob) (string, error) {
		return "", &exec.Error{Name: "delta", Err: exec.ErrNotFound}
	}
	renders.reset()
	m.ClearCache()
	if _, ok := runCmd(t, m.SetSideBySide(true)).(common.ErrMsg); !ok {
		t.Error("a missing delta should be reported as an error")
	}
}

// A watch reload whose new output gains a commit header: the viewer is
// cleared, resized (the header takes rows) and given the new diff, in
// either order; the new diff must be what lands.
func TestWatchReloadWithHeaderChangeShowsNewDiff(t *testing.T) {
	for _, resizeFirst := range []bool{true, false} {
		useFakeDelta(t)
		m := newRenderTestModel(t, "tokyo_night", 100, 30)
		m, cmd := m.SetDirPatch("/", testFiles(t, 2, 1))
		m = land(t, m, cmd)

		newFiles := testFiles(t, 4, 1)
		m.ClearCache()
		m.SetPreamble("commit abc123")
		var cmds []tea.Cmd
		if resizeFirst {
			cmds = append(cmds, m.SetSize(100, 27))
		}
		m, cmd = m.SetDirPatch("/", newFiles)
		cmds = append(cmds, cmd)
		if !resizeFirst {
			cmds = append(cmds, m.SetSize(100, 27))
		}
		for _, c := range cmds {
			m = land(t, m, c)
		}
		if s := shownText(m); !strings.Contains(s, "f3.txt") || !strings.Contains(s, "commit abc123") {
			t.Errorf("resizeFirst=%v: the old diff is still shown:\n%s", resizeFirst, s)
		}
	}
}
