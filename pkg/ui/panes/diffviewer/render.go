package diffviewer

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"

	"github.com/dlvhdr/diffnav/pkg/filenode"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

// maxParallelDeltas caps the delta processes a render runs at once.
const maxParallelDeltas = 8

// deltaJob is one delta process's worth of work. It is built on the main
// loop (its args read the theme and the styles, which the theme picker
// changes there) and run off it.
type deltaJob struct {
	args       []string
	sideBySide bool // false: unified, forced through deltaEnv
	input      string
}

// renderRequest is everything a render needs, captured when it is asked
// for: running it touches nothing in the Model.
type renderRequest struct {
	gen   uint64
	key   string
	path  string
	isDir bool
	// head is the rendered preamble ("" or ending in a newline; the
	// root directory view only).
	head  string
	jobs  []deltaJob
	paths []string // the files' names (directory views)
}

type diffContentMsg struct {
	gen        uint64
	key        string
	path       string
	isDir      bool
	lines      []diffLine
	fileStarts []int
	filePaths  []string
}

// newRender asks for a render of node under key: one delta per file. Each
// file in a directory gets the layout its single-file view would (a new or
// deleted file is one column, not side-by-side with an empty half); one
// delta per file (instead of one for the whole directory) tells where each
// file starts, which the file tree follows.
func (m *Model) newRender(node *cachedNode, dir bool, key string) renderRequest {
	width := m.contentWidth()
	req := renderRequest{gen: m.gen, key: key, path: node.path, isDir: dir}
	if !dir {
		file := node.files[0]
		sbs := m.sideBySide && !file.IsNew && !file.IsDelete
		req.jobs = []deltaJob{{
			args:       m.makeDeltaArgs(sbs, width, singleFileOpts(file)),
			sideBySide: sbs,
			input:      file.String() + "\n",
		}}
		return req
	}

	type layout struct {
		args []string
		sbs  bool
	}
	layouts := map[dirFileKind]layout{}
	for _, kind := range []dirFileKind{dirFileChanged, dirFileNew, dirFileDeleted} {
		sbs, opts := kind.deltaLayout(m.sideBySide)
		layouts[kind] = layout{m.makeDeltaArgs(sbs, width, opts), sbs}
	}
	req.jobs = make([]deltaJob, len(node.files))
	req.paths = make([]string, len(node.files))
	for i, file := range node.files {
		l := layouts[dirFileKindOf(file)]
		req.jobs[i] = deltaJob{args: l.args, sideBySide: l.sbs, input: file.String() + "\n"}
		req.paths[i] = filenode.GetFileName(file)
	}
	if node.path == "/" && m.preamble != "" {
		req.head = m.renderPreamble(m.preamble) + "\n"
	}
	return req
}

// cmd runs the render off the main loop.
func (r renderRequest) cmd() tea.Cmd {
	if len(r.jobs) > 0 {
		log.Info("executing delta", "files", len(r.jobs), "cmd", fmt.Sprintf(`delta %s`, strings.Join(r.jobs[0].args, " ")))
	}
	return func() tea.Msg {
		outs, err := renderFiles(len(r.jobs), func(i int) (string, error) {
			return runDelta(r.jobs[i])
		})
		if err != nil {
			if stdErr, ok := err.(*exec.ExitError); ok {
				return common.ErrMsg{Err: err, Message: string(stdErr.Stderr)}
			}
			log.Warn("delta error", "err", err)
			return common.ErrMsg{Err: err}
		}
		msg := diffContentMsg{gen: r.gen, key: r.key, path: r.path, isDir: r.isDir}
		if !r.isDir {
			msg.lines = stringToDiffLines(outs[0])
			return msg
		}
		text, starts := joinFileRenders(r.head, outs)
		msg.lines = stringToDiffLines(text)
		msg.fileStarts = starts
		msg.filePaths = r.paths
		return msg
	}
}

// runDelta runs delta for job and returns its output. A var so tests can
// stand in for delta.
var runDelta = func(job deltaJob) (string, error) {
	deltac := exec.Command("delta", job.args...)
	deltac.Env = deltaEnv(os.Environ(), job.sideBySide)
	deltac.Stdin = strings.NewReader(job.input)
	out, err := deltac.Output()
	return string(out), err
}

// renderFiles runs render for 0..n-1, at most maxParallelDeltas at a time,
// and returns the outputs in order (or the first error).
func renderFiles(n int, render func(i int) (string, error)) ([]string, error) {
	outs := make([]string, n)
	errs := make([]error, n)
	sem := make(chan struct{}, maxParallelDeltas)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			outs[i], errs[i] = render(i)
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
