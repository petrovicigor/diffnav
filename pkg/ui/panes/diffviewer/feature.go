package diffviewer

import (
	"bytes"
	"fmt"
	"image/color"
	"os/exec"
	"regexp"
	"sync"

	"charm.land/log/v2"
)

// hasDeltaFeature reports whether the gitconfig delta reads defines the delta
// feature name ([delta "name"]). delta silently ignores an unknown
// --features, so a Miasma render without the user's miasma-dark /
// miasma-light features came out in delta's default colours (unreadable on
// the light theme); with no feature, diffnav styles delta itself instead.
// A var so tests can pin it.
var hasDeltaFeature = gitHasDeltaFeature

var deltaFeatures sync.Map // name → bool, looked up once per process

// gitHasDeltaFeature asks git, once per name: git config reads the same
// files delta does (system, global, the repo's own, includes resolved), from
// the same working directory.
func gitHasDeltaFeature(name string) bool {
	if v, ok := deltaFeatures.Load(name); ok {
		return v.(bool)
	}
	out, err := exec.Command(
		"git", "config", "--get-regexp", `^delta\.`+regexp.QuoteMeta(name)+`\.`,
	).Output()
	found := err == nil && len(bytes.TrimSpace(out)) > 0
	if !found {
		log.Warn("delta feature not in gitconfig, using diffnav's own delta styles", "feature", name)
	}
	deltaFeatures.Store(name, found)
	return found
}

// mixHex is the colour t of the way from a to b, as #rrggbb.
func mixHex(a, b color.Color, t float64) string {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 {
		return uint8((float64(x>>8)*(1-t) + float64(y>>8)*t) + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
}
