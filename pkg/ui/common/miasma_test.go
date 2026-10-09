package common

import "testing"

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
	}
}
