package diffviewer

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDeltaEnv(t *testing.T) {
	base := []string{"HOME=/home/u", "PATH=/bin"}
	if got := deltaEnv(base, true); !slices.Equal(got, base) {
		t.Errorf("side-by-side must keep the environment: %q", got)
	}
	want := append(slices.Clone(base), "GIT_CONFIG_PARAMETERS='delta.side-by-side'='false'")
	if got := deltaEnv(base, false); !slices.Equal(got, want) {
		t.Errorf("unified: got %q, want %q", got, want)
	}
	withParams := []string{"GIT_CONFIG_PARAMETERS='core.pager'='less'", "HOME=/home/u"}
	got := deltaEnv(withParams, false)
	want = []string{"HOME=/home/u", "GIT_CONFIG_PARAMETERS='core.pager'='less' 'delta.side-by-side'='false'"}
	if !slices.Equal(got, want) {
		t.Errorf("existing params must be kept, ours last: got %q, want %q", got, want)
	}
	if base[0] != "HOME=/home/u" || len(base) != 2 {
		t.Errorf("deltaEnv must not modify its input: %q", base)
	}
}

// A gitconfig with delta.side-by-side = true (the user's) must not make a
// unified render side-by-side, while the gitconfig's features still load.
func TestDeltaEnv_BeatsSideBySideInGitconfig(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed")
	}
	home := t.TempDir()
	gitconfig := "[delta]\n\tside-by-side = true\n[delta \"probe\"]\n\tfile-modified-label = PROBE\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	// Similar enough that side-by-side pairs them on one row.
	diff := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-hello world\n+hello there world\n"
	environ := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"GIT_CONFIG_NOSYSTEM=1",
		"PATH=" + os.Getenv("PATH"),
	}
	run := func(env []string, args ...string) string {
		cmd := exec.Command("delta", append([]string{"--paging=never", "--features=probe", "-w=100"}, args...)...)
		cmd.Env = env
		cmd.Dir = home // no repo config
		cmd.Stdin = strings.NewReader(diff)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("delta %q: %v", args, err)
		}
		return string(out)
	}
	// Side-by-side puts the old and the new line on one row.
	sideBySide := func(out string) bool {
		for l := range strings.SplitSeq(ansi.Strip(out), "\n") {
			if strings.Contains(l, "hello world") && strings.Contains(l, "hello there world") {
				return true
			}
		}
		return false
	}
	if !sideBySide(run(environ)) {
		t.Fatal("precondition: the probe gitconfig should make delta side-by-side")
	}
	out := run(deltaEnv(environ, false))
	if sideBySide(out) {
		t.Errorf("unified env still renders side-by-side:\n%s", out)
	}
	if !strings.Contains(out, "PROBE") {
		t.Errorf("features from the gitconfig were lost:\n%s", out)
	}
	if !sideBySide(run(deltaEnv(environ, false), "--side-by-side")) {
		t.Error("--side-by-side must still win over the unified env")
	}
}
