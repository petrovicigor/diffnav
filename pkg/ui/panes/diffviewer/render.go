package diffviewer

import (
	"context"
	"crypto/sha256"
	"errors"
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

// maxParallelDeltas caps the delta processes running at once (deltaSlots).
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

// cmd runs the render off the main loop. A render cancelled by ctx (a newer
// request superseded it) sends nothing.
func (r renderRequest) cmd(ctx context.Context) tea.Cmd {
	if len(r.jobs) > 0 {
		log.Info("executing delta", "files", len(r.jobs), "cmd", fmt.Sprintf(`delta %s`, strings.Join(r.jobs[0].args, " ")))
	}
	return func() tea.Msg {
		outs, err := renderFiles(ctx, len(r.jobs), func(ctx context.Context, i int) (string, error) {
			out, err := renderJob(ctx, r.jobs[i])
			if err != nil && ctx.Err() == nil && !errors.Is(err, exec.ErrNotFound) {
				// One file delta chokes on costs that file, not the view.
				path := r.path
				if r.isDir {
					path = r.paths[i]
				}
				log.Warn("delta failed", "file", path, "err", err)
				return failedRender(path, r.jobs[i].input, err), nil
			}
			return out, err
		})
		if ctx.Err() != nil {
			return nil
		}
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

// deltaSlots caps the delta processes running at once in the whole
// program, across every render: a resize storm or a theme preview spree
// starts new renders faster than old ones finish (and those are cancelled).
var deltaSlots = make(chan struct{}, maxParallelDeltas)

// renderJob is job's delta output: from the render cache, else from a delta
// run once a slot is free. It gives up (ctx's error) as soon as ctx is
// cancelled, killing a delta already running.
func renderJob(ctx context.Context, job deltaJob) (string, error) {
	key := job.cacheKey()
	if out, ok := renders.get(key); ok {
		return out, nil
	}
	select {
	case deltaSlots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-deltaSlots }()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	out, err := runDelta(ctx, job)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	renders.put(key, out)
	return out, nil
}

// runDelta runs delta for job and returns its output; cancelling ctx kills
// it. A var so tests can stand in for delta.
var runDelta = func(ctx context.Context, job deltaJob) (string, error) {
	deltac := exec.CommandContext(ctx, "delta", job.args...)
	deltac.Env = deltaEnv(os.Environ(), job.sideBySide)
	deltac.Stdin = strings.NewReader(job.input)
	out, err := deltac.Output()
	return string(out), err
}

// renderFiles runs render for 0..n-1 in parallel (deltaSlots bounds the
// processes) and returns the outputs in order, or the first error, or
// ctx's error once it is cancelled.
func renderFiles(ctx context.Context, n int, render func(ctx context.Context, i int) (string, error)) ([]string, error) {
	outs := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outs[i], errs[i] = render(ctx, i)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return outs, nil
}

// cacheKey identifies job's output: everything delta is given.
func (j deltaJob) cacheKey() [sha256.Size]byte {
	h := sha256.New()
	for _, a := range j.args {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	if j.sideBySide {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{2})
	}
	h.Write([]byte(j.input))
	var sum [sha256.Size]byte
	h.Sum(sum[:0])
	return sum
}

// renderCacheMaxBytes bounds the render cache; past it the cache starts
// over (renders of other widths and themes are the bulk of it by then).
const renderCacheMaxBytes = 64 << 20

// renderCache holds every file's delta output by its job (args and input):
// shared by all views, so toggles, theme flips and returning to a width
// re-run delta only for what changed.
type renderCache struct {
	mu    sync.Mutex
	outs  map[[sha256.Size]byte]string
	bytes int
}

var renders = &renderCache{}

func (c *renderCache) get(key [sha256.Size]byte) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out, ok := c.outs[key]
	return out, ok
}

func (c *renderCache) put(key [sha256.Size]byte, out string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.outs == nil || c.bytes+len(out) > renderCacheMaxBytes {
		c.outs = map[[sha256.Size]byte]string{}
		c.bytes = 0
	}
	if _, ok := c.outs[key]; !ok {
		c.bytes += len(out)
	}
	c.outs[key] = out
}

func (c *renderCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outs, c.bytes = nil, 0
}

// IsRenderMsg reports whether msg is the viewer's own render traffic, which
// must reach it whatever has the keyboard (the file search included).
func IsRenderMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case diffContentMsg, renderDueMsg:
		return true
	}
	return false
}

// failedRender stands in for the render of a file delta failed on: a note
// with delta's complaint, then the file's plain patch.
func failedRender(path, input string, err error) string {
	why := err.Error()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		why, _, _ = strings.Cut(strings.TrimSpace(string(exitErr.Stderr)), "\n")
	}
	return fmt.Sprintf("⚠ delta failed on %s: %s\n%s", path, why, input)
}
