package diffviewer

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

// pinDeltaFeatures makes hasDeltaFeature answer has for the test's duration.
func pinDeltaFeatures(t *testing.T, has bool) {
	t.Helper()
	prev := hasDeltaFeature
	hasDeltaFeature = func(string) bool { return has }
	t.Cleanup(func() { hasDeltaFeature = prev })
}

// Without the gitconfig's miasma features (another machine), Miasma falls
// back to diffnav's own delta styling instead of delta's defaults.
func TestMakeDeltaArgs_MiasmaWithoutFeatureFallsBack(t *testing.T) {
	pinDeltaFeatures(t, false)
	common.RegisterSupportedTints()
	for id, want := range map[string][]string{
		"miasma_dark":  {"--syntax-theme=gruvbox-dark", "--dark"},
		"miasma_light": {"--syntax-theme=gruvbox-light", "--light"},
	} {
		common.Themes.SetTintID(id)
		styles := common.MakeStyles()
		m := New(true, &styles)
		args := m.makeDeltaArgs(true, 120, deltaOpts{})
		if slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--features") }) {
			t.Errorf("%s: no feature in the gitconfig, yet passed one: %q", id, args)
		}
		for _, w := range want {
			if !slices.Contains(args, w) {
				t.Errorf("%s: fallback styling lacks %q: %q", id, w, args)
			}
		}
		if !slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--plus-style=") }) {
			t.Errorf("%s: fallback must style +/- lines itself: %q", id, args)
		}
	}
}

func TestGitHasDeltaFeature(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	gitconfig := "[delta \"probe-feature\"]\n\tdark = true\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Chdir(home)
	if !gitHasDeltaFeature("probe-feature") {
		t.Error("defined feature not found")
	}
	if gitHasDeltaFeature("probe-feature-missing") {
		t.Error("missing feature reported as present")
	}
	if gitHasDeltaFeature("probe") {
		t.Error("a feature name must match whole, not as a prefix")
	}
}

func TestMakeDeltaArgs_MiasmaUsesGitconfigFeature(t *testing.T) {
	pinDeltaFeatures(t, true)
	common.RegisterSupportedTints()
	for id, feature := range map[string]string{
		"miasma_dark":  "miasma-dark",
		"miasma_light": "miasma-light",
	} {
		common.Themes.SetTintID(id)
		styles := common.MakeStyles()
		m := New(true, &styles)

		for _, opts := range []deltaOpts{{}, {IsNew: true}, {IsDelete: true}} {
			args := m.makeDeltaArgs(true, 120, opts)
			if !slices.Contains(args, "--features="+feature) {
				t.Errorf("%s %+v: missing --features=%s in %q", id, opts, feature, args)
			}
			for _, a := range args {
				if slices.Contains(featureHeaderArgs, a) {
					continue // header text, no colour
				}
				if a == "--hunk-header-decoration-style="+hunkRuleColor(feature)+" ol" {
					continue // the one deliberate override: diffnav's own rule
				}
				flag, _, _ := strings.Cut(a, "=")
				if strings.HasSuffix(flag, "-style") || strings.HasSuffix(flag, "-label") ||
					flag == "--syntax-theme" || flag == "--dark" || flag == "--light" {
					t.Errorf("%s %+v: colour/style arg %q leaked into %q", id, opts, a, args)
				}
			}
			for _, want := range []string{"--paging=never", "--line-numbers", "-w=120", "--side-by-side"} {
				if !slices.Contains(args, want) {
					t.Errorf("%s %+v: missing layout arg %q in %q", id, opts, want, args)
				}
			}
		}
	}
}

func TestFeatureHeaderArgs_NoColourNoNavigateMarkers(t *testing.T) {
	for _, a := range featureHeaderArgs {
		flag, val, _ := strings.Cut(a, "=")
		switch {
		case strings.HasSuffix(flag, "-label"):
			if val != "" {
				t.Errorf("%s: labels must be empty (no Δ / added: / •), got %q", flag, val)
			}
		case flag == "--hunk-header-style":
			if val != "line-number syntax" {
				t.Errorf("hunk header must be line number + code only (no file, no colour), got %q", val)
			}
		default:
			t.Errorf("unexpected header arg %q", a)
		}
	}
	for _, label := range []string{"modified", "added", "removed", "renamed", "copied"} {
		if !slices.Contains(featureHeaderArgs, "--file-"+label+"-label=") {
			t.Errorf("navigate's %s label is not cleared", label)
		}
	}
}

func TestSingleFileOpts_OmitFileHeaderUnlessRenamed(t *testing.T) {
	cases := []struct {
		file *gitdiff.File
		omit bool
	}{
		{&gitdiff.File{OldName: "a.go", NewName: "a.go"}, true},
		{&gitdiff.File{NewName: "a.go", IsNew: true}, true},
		{&gitdiff.File{OldName: "a.go", IsDelete: true}, true},
		{&gitdiff.File{OldName: "a.go", NewName: "b.go", IsRename: true}, false},
		{&gitdiff.File{OldName: "a.go", NewName: "b.go", IsCopy: true}, false},
	}
	for _, c := range cases {
		opts := singleFileOpts(c.file)
		if opts.OmitFileHeader != c.omit {
			t.Errorf("%s→%s: OmitFileHeader = %v, want %v", c.file.OldName, c.file.NewName, opts.OmitFileHeader, c.omit)
		}
		args := featureDeltaArgs("miasma-dark", true, 100, opts)
		if got := slices.Contains(args, "--file-style=omit"); got != c.omit {
			t.Errorf("%s→%s: --file-style=omit present = %v, want %v", c.file.OldName, c.file.NewName, got, c.omit)
		}
	}
	// A directory view keeps every file's header.
	if slices.Contains(featureDeltaArgs("miasma-dark", true, 100, deltaOpts{}), "--file-style=omit") {
		t.Error("directory view must keep delta's file headers")
	}
}

func TestMakeDeltaArgs_OtherThemesKeepDiffnavStyling(t *testing.T) {
	common.RegisterSupportedTints()
	common.Themes.SetTintID("tokyo_night")
	styles := common.MakeStyles()
	m := New(false, &styles)
	args := m.makeDeltaArgs(false, 80, deltaOpts{})
	for _, a := range args {
		if strings.HasPrefix(a, "--features") {
			t.Fatalf("non-Miasma theme should not pass a feature: %q", args)
		}
	}
	if !slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--syntax-theme=") }) {
		t.Fatalf("non-Miasma theme lost its syntax theme: %q", args)
	}
}

func TestDirFileKind_NewAndDeletedFilesAreOneColumn(t *testing.T) {
	cases := []struct {
		file     *gitdiff.File
		sbs      bool
		new, del bool
	}{
		{&gitdiff.File{OldName: "a.go", NewName: "a.go"}, true, false, false},
		{&gitdiff.File{NewName: "a.go", IsNew: true}, false, true, false},
		{&gitdiff.File{OldName: "a.go", IsDelete: true}, false, false, true},
	}
	for _, c := range cases {
		sbs, opts := dirFileKindOf(c.file).deltaLayout(true)
		if sbs != c.sbs || opts.IsNew != c.new || opts.IsDelete != c.del {
			t.Errorf("%+v: got sbs=%v opts=%+v", c.file, sbs, opts)
		}
		if opts.OmitFileHeader {
			t.Errorf("%+v: a directory view must keep the file header", c.file)
		}
	}
	if sbs, _ := dirFileKindOf(&gitdiff.File{OldName: "a", NewName: "a"}).deltaLayout(false); sbs {
		t.Error("unified mode must stay unified")
	}
}
