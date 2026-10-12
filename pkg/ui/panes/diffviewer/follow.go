package diffviewer

import (
	"sort"
	"strings"
)

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

// fileJump is the last JumpFile: the file it went to, the render it was in
// (its key), and the top line it left the viewport at.
type fileJump struct {
	ok  bool
	key string
	idx int
	top int
}

// currentFileIdx is the file shown at top. A short file at the end of the
// diff can never reach the top line (the viewport stops at the bottom), so a
// jump to it stays current for as long as the viewport has not moved since.
func currentFileIdx(starts []int, top int, j fileJump, key string) int {
	if j.ok && j.key == key && j.top == top && j.idx >= 0 && j.idx < len(starts) {
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

// followable is the directory render on screen whose files the tree can
// follow: nil in a single-file view, before the asked-for directory's
// render landed (the screen still shows another node), or while a filter
// shows only matching lines (line numbers no longer map to files).
func (m *Model) followable() *cachedNode {
	node := m.shown
	if m.dir == nil || node == nil || !node.isDir || node.path != m.dir.path || len(node.fileStarts) == 0 {
		return nil
	}
	if m.fvp.GetMatchingItemsOnly() && m.Filtering() {
		return nil
	}
	return node
}

func (m *Model) topLine() int {
	return m.fvp.GetItemMetrics().FirstVisibleItemIdx
}

// focusLine is the line the view is "at": the selected line with selection
// on (j/k move it, not the viewport), else the viewport's top line. -1 for
// none.
func (m *Model) focusLine() int {
	if m.fvp.GetSelectionEnabled() {
		if m.fvp.GetItemMetrics().TotalItems == 0 {
			return -1
		}
		return m.fvp.GetSelectedItemIdx()
	}
	return m.topLine()
}

// CurrentFilePath is the path of the file at a directory view's focus line
// (the top line, or the selected one) — "" in a single-file view or above
// the first file.
func (m *Model) CurrentFilePath() string {
	node := m.followable()
	if node == nil {
		return ""
	}
	line := m.focusLine()
	if line < 0 {
		return ""
	}
	idx := currentFileIdx(node.fileStarts, line, m.jump, node.key)
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

// JumpFile scrolls a directory view so the next (delta > 0) or previous
// (delta < 0) file starts at the top; with selection on it also selects
// that first line. It reports whether it moved.
func (m *Model) JumpFile(delta int) bool {
	node := m.followable()
	if node == nil {
		return false
	}
	line := m.focusLine()
	if line < 0 {
		return false
	}
	cur := currentFileIdx(node.fileStarts, line, m.jump, node.key)
	target := jumpTarget(node.fileStarts, line, cur, delta)
	if target < 0 {
		return false
	}
	start := node.fileStarts[target]
	if m.fvp.GetSelectionEnabled() {
		// With selection on, scrolling moves the selection, which brought
		// the file only to the bottom edge: scroll with it off, then select.
		m.fvp.SetSelectionEnabled(false)
		m.scrollTopTo(start)
		m.fvp.SetSelectionEnabled(true)
		m.fvp.SetSelectedItemIdx(start) // a short last file can't reach the top
	} else {
		m.scrollTopTo(start)
	}
	m.jump = fileJump{ok: true, key: node.key, idx: target, top: m.focusLine()}
	return true
}

// scrollTopTo puts line at the viewport's top, or as near as the end of the
// diff allows (selection off).
func (m *Model) scrollTopTo(line int) {
	m.fvp.GoToTop()
	m.fvp.ScrollDown(line)
}
