package diffviewer

import (
	"slices"
	"strings"
	"testing"

	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

func TestMakeDeltaArgs_MiasmaUsesGitconfigFeature(t *testing.T) {
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
