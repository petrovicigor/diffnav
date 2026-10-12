package common

import (
	"image/color"
	"math"
	"testing"
)

func TestResolveThemeID(t *testing.T) {
	env := func(mode string) func(string) string {
		return func(k string) string {
			if k == "MIASMA_MODE" {
				return mode
			}
			return ""
		}
	}
	cases := []struct {
		id, mode, want string
		auto           bool
	}{
		{"miasma", "light", "miasma_light", false},
		{"miasma", "dark", "miasma_dark", false},
		{"miasma", "", "miasma_dark", true},
		{"miasma", "sepia", "miasma_dark", true},
		{"tokyo_night", "light", "tokyo_night", false},
		{"miasma_light", "dark", "miasma_light", false},
	}
	for _, c := range cases {
		got, auto := ResolveThemeID(c.id, env(c.mode))
		if got != c.want || auto != c.auto {
			t.Errorf("ResolveThemeID(%q, MIASMA_MODE=%q) = %q, %v; want %q, %v",
				c.id, c.mode, got, auto, c.want, c.auto)
		}
	}
}

func TestMiasmaTintsRegisteredAndStyled(t *testing.T) {
	RegisterSupportedTints()
	for _, id := range []string{"miasma_dark", "miasma_light"} {
		if !Themes.SetTintID(id) {
			t.Fatalf("%s is not registered", id)
		}
		s := MakeStyles()
		if !IsMiasma(s.Tint) {
			t.Fatalf("%s: IsMiasma = false", id)
		}
		mc := miasmaDarkColors
		if id == "miasma_light" {
			mc = miasmaLightColors
		}
		if got := LipglossColorToHex(s.Colors.SelectionBg); got != mc.selectionBg {
			t.Errorf("%s selection bg = %s, want %s", id, got, mc.selectionBg)
		}
		if got := LipglossColorToHex(s.Colors.CurrentBg); got != mc.currentBg {
			t.Errorf("%s current bg = %s, want %s", id, got, mc.currentBg)
		}
		// The scroll pill's text must read on its BrightBlack body.
		if r := contrast(s.Colors.PillFg, s.Tint.BrightBlack); r < 2.8 {
			t.Errorf("%s scroll pill contrast %.2f:1, want ≥ 2.8", id, r)
		}
	}
}

// contrast is the WCAG contrast ratio of a and b.
func contrast(a, b color.Color) float64 {
	lum := func(c color.Color) float64 {
		r, g, b, _ := c.RGBA()
		ch := func(v uint32) float64 {
			s := float64(v) / 0xffff
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(r) + 0.7152*ch(g) + 0.0722*ch(b)
	}
	la, lb := lum(a), lum(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}
