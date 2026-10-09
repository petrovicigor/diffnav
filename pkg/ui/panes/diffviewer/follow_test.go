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
	j := fileJump{ok: true, path: "/", idx: 2, top: 80}
	if got := currentFileIdx(starts, 80, j, "/"); got != 2 {
		t.Errorf("right after the jump: got %d, want 2", got)
	}
	if got := currentFileIdx(starts, 79, j, "/"); got != 1 {
		t.Errorf("after scrolling: got %d, want 1", got)
	}
	if got := currentFileIdx(starts, 80, j, "/app"); got != 1 {
		t.Errorf("in another directory view: got %d, want 1", got)
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

func TestRenderFilesKeepsOrder(t *testing.T) {
	files := make([]*gitdiff.File, 30)
	for i := range files {
		files[i] = &gitdiff.File{NewName: fmt.Sprintf("f%02d", i)}
	}
	outs, err := renderFiles(files, func(f *gitdiff.File) (string, error) {
		return f.NewName + "\n", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, out := range outs {
		if want := fmt.Sprintf("f%02d\n", i); out != want {
			t.Fatalf("outs[%d] = %q, want %q", i, out, want)
		}
	}

	_, err = renderFiles(files, func(f *gitdiff.File) (string, error) {
		if f.NewName == "f07" {
			return "", fmt.Errorf("boom")
		}
		return "", nil
	})
	if err == nil {
		t.Fatal("expected the render error")
	}
}
