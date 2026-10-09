package diffviewer

import (
	"sort"
	"strings"
	"sync"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// maxParallelDeltas caps the delta processes a directory render runs at once.
const maxParallelDeltas = 8

// renderFiles renders every file with render, at most maxParallelDeltas at a
// time, and returns the outputs in files' order (or the first error).
func renderFiles(files []*gitdiff.File, render func(*gitdiff.File) (string, error)) ([]string, error) {
	outs := make([]string, len(files))
	errs := make([]error, len(files))
	sem := make(chan struct{}, maxParallelDeltas)
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			outs[i], errs[i] = render(f)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return outs, nil
}

// joinFileRenders concatenates head (the preamble, "" or ending in a
// newline) and each file's render into one text, and returns the line each
// file starts at. A render missing its final newline gets one, so every file
// starts on a line of its own.
func joinFileRenders(head string, renders []string) (string, []int) {
	var b strings.Builder
	b.WriteString(head)
	line := strings.Count(head, "\n")
	starts := make([]int, len(renders))
	for i, r := range renders {
		starts[i] = line
		if r != "" && !strings.HasSuffix(r, "\n") {
			r += "\n"
		}
		b.WriteString(r)
		line += strings.Count(r, "\n")
	}
	return b.String(), starts
}

// fileIndexAt is the file whose render holds line: the last file starting at
// or before it, or -1 when line comes before the first file (the preamble).
func fileIndexAt(starts []int, line int) int {
	return sort.Search(len(starts), func(i int) bool { return starts[i] > line }) - 1
}

// fileJump is the last JumpFile: the file it went to, the directory view it
// was in, and the top line it left the viewport at.
type fileJump struct {
	ok   bool
	path string
	idx  int
	top  int
}

// currentFileIdx is the file shown at top. A short file at the end of the
// diff can never reach the top line (the viewport stops at the bottom), so a
// jump to it stays current for as long as the viewport has not moved since.
func currentFileIdx(starts []int, top int, j fileJump, path string) int {
	if j.ok && j.path == path && j.top == top && j.idx >= 0 && j.idx < len(starts) {
		return j.idx
	}
	return fileIndexAt(starts, top)
}

// jumpTarget is the file a jump by delta (+1 next, -1 previous) goes to from
// the current file cur with top as the viewport's top line, or -1 for none.
// Previous first returns to the start of cur when top is inside it, the way
// vim's [[ does.
func jumpTarget(starts []int, top, cur, delta int) int {
	var target int
	switch {
	case delta > 0:
		target = cur + 1
	case cur < 0:
		return -1
	case top > starts[cur]:
		target = cur
	default:
		target = cur - 1
	}
	if target < 0 || target >= len(starts) {
		return -1
	}
	return target
}

// followable is the directory view whose files the tree can follow: nil in
// a single-file view, before its render landed, or while a filter shows
// only matching lines (line numbers no longer map to files).
func (m *Model) followable() *cachedNode {
	if m.dir == nil || len(m.dir.fileStarts) == 0 {
		return nil
	}
	if m.fvp.GetMatchingItemsOnly() && m.Filtering() {
		return nil
	}
	return m.dir
}

func (m *Model) topLine() int {
	return m.fvp.GetItemMetrics().FirstVisibleItemIdx
}

// CurrentFilePath is the path of the file at the top of a directory view's
// viewport ("" in a single-file view or above the first file).
func (m *Model) CurrentFilePath() string {
	node := m.followable()
	if node == nil {
		return ""
	}
	top := m.topLine()
	if top < 0 {
		return ""
	}
	idx := currentFileIdx(node.fileStarts, top, m.jump, node.path)
	if idx < 0 || idx >= len(node.filePaths) {
		return ""
	}
	return node.filePaths[idx]
}

// IsDirView reports whether the viewer shows a directory (all its files in
// one scroll) rather than a single file.
func (m *Model) IsDirView() bool {
	return m.dir != nil
}

// JumpFile scrolls a directory view to the start of the next (delta > 0) or
// previous (delta < 0) file. It reports whether it moved.
func (m *Model) JumpFile(delta int) bool {
	node := m.followable()
	if node == nil {
		return false
	}
	top := m.topLine()
	if top < 0 {
		return false
	}
	cur := currentFileIdx(node.fileStarts, top, m.jump, node.path)
	target := jumpTarget(node.fileStarts, top, cur, delta)
	if target < 0 {
		return false
	}
	m.fvp.GoToTop()
	m.fvp.ScrollDown(node.fileStarts[target])
	m.jump = fileJump{ok: true, path: node.path, idx: target, top: m.topLine()}
	return true
}
