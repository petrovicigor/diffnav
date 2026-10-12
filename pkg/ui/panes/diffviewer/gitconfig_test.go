package diffviewer

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestUnifiedGitconfigText(t *testing.T) {
	got := unifiedGitconfigText("/home/u", "")
	for _, want := range []string{
		"path = /home/u/.config/git/config",
		"path = /home/u/.gitconfig",
		"[delta]\n\tside-by-side = false\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	// side-by-side = false must come after the includes to win.
	if strings.Index(got, "side-by-side") < strings.Index(got, ".gitconfig") {
		t.Errorf("side-by-side must follow the includes:\n%s", got)
	}
	if got := unifiedGitconfigText("/home/u", "/xdg"); !strings.Contains(got, "path = /xdg/git/config") {
		t.Errorf("XDG_CONFIG_HOME ignored:\n%s", got)
	}
}

func TestWithConfig(t *testing.T) {
	if got := withConfig([]string{"-w=80"}, ""); !slices.Equal(got, []string{"-w=80"}) {
		t.Errorf("no path: got %q", got)
	}
	if got := withConfig([]string{"-w=80"}, "/tmp/x"); !slices.Equal(got, []string{"-w=80", "--config=/tmp/x"}) {
		t.Errorf("path: got %q", got)
	}
}

// A gitconfig with delta.side-by-side = true (the user's) must not make a
// unified render side-by-side, while its features still load.
func TestUnifiedGitconfig_BeatsSideBySideInGitconfig(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed")
	}
	home := t.TempDir()
	gitconfig := "[delta]\n\tside-by-side = true\n[delta \"probe\"]\n\tfile-modified-label = PROBE\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "unified.gitconfig")
	if err := os.WriteFile(cfg, []byte(unifiedGitconfigText(home, filepath.Join(home, "xdg"))), 0o600); err != nil {
		t.Fatal(err)
	}
	diff := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"
	run := func(args ...string) string {
		cmd := exec.Command("delta", append([]string{"--paging=never", "--line-numbers", "--features=probe", "-w=60"}, args...)...)
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"))
		cmd.Stdin = strings.NewReader(diff)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("delta %q: %v", args, err)
		}
		return string(out)
	}
	// Side-by-side draws the removed line as "│  1 │old   │    │" (two
	// panels, four bars); unified draws one number column, one bar.
	sideBySideLine := func(out string) bool {
		for l := range strings.SplitSeq(out, "\n") {
			if strings.Contains(l, "old") && strings.Count(l, "│") >= 3 {
				return true
			}
		}
		return false
	}
	if !sideBySideLine(run()) {
		t.Fatal("precondition: the probe gitconfig should make delta side-by-side")
	}
	out := run("--config=" + cfg)
	if sideBySideLine(out) {
		t.Errorf("unified config still renders side-by-side:\n%s", out)
	}
	if !strings.Contains(out, "PROBE") {
		t.Errorf("features from the included gitconfig were lost:\n%s", out)
	}
	if !sideBySideLine(run("--config="+cfg, "--side-by-side")) {
		t.Error("--side-by-side must still win over the unified config")
	}
}

func TestUnifiedGitconfigPath_NamedByContent(t *testing.T) {
	a := unifiedGitconfigPath("/tmp", unifiedGitconfigText("/home/a", ""))
	if a != unifiedGitconfigPath("/tmp", unifiedGitconfigText("/home/a", "")) {
		t.Error("same content must reuse one file")
	}
	if a == unifiedGitconfigPath("/tmp", unifiedGitconfigText("/home/b", "")) {
		t.Error("another home must get its own file")
	}
	if filepath.Dir(a) != "/tmp" || !strings.HasPrefix(filepath.Base(a), "diffnav-unified-") {
		t.Errorf("unexpected path %q", a)
	}
}
