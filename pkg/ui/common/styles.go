package common

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
	tint "github.com/lrstanley/bubbletint/v2"
)

// lipglossColorToHex converts a color.Color to hex string
func LipglossColorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

type Styles struct {
	Tint   *tint.Tint
	Colors Colors
}

type Colors struct {
	SelectionFg       color.Color
	SelectionBg       color.Color
	DarkerSelectionBg color.Color
	FaintBlue         func() color.Color
	// CurrentBg marks the file at the top of the diff viewport in the file
	// tree (distinct from the cursor's SelectionBg).
	CurrentBg color.Color
	// Search match highlights in the diff viewer.
	MatchFg, MatchBg               color.Color
	MatchFocusedFg, MatchFocusedBg color.Color
	// PillFg is the text of the diff viewer's scroll pill ("26% (38/142)").
	PillFg color.Color
}

func MakeStyles() Styles {
	t := Themes.Current()
	if t.ID == tint.TintTokyoNightStorm.ID {
		t.BrightGreen = tint.FromHex("#9ece6a")
	}

	selectionFg := lipgloss.Darken(t.Blue, 0.3)
	selectionBg := lipgloss.Darken(t.Blue, 0.6)
	colors := Colors{
		SelectionFg:       selectionFg,
		SelectionBg:       selectionBg,
		DarkerSelectionBg: lipgloss.Darken(selectionBg, 0.3),
		FaintBlue: func() color.Color {
			return lipgloss.Darken(t.Blue, 0.8)
		},
		CurrentBg:      lipgloss.Darken(t.Green, 0.6),
		MatchFg:        lipgloss.Lighten(t.BrightWhite, 0.5),
		MatchBg:        lipgloss.Lighten(selectionBg, 0.1),
		MatchFocusedFg: lipgloss.Lighten(t.Yellow, 0.5),
		MatchFocusedBg: lipgloss.Darken(t.Yellow, 0.2),
		PillFg:         t.White,
	}

	if IsMiasma(t) {
		mc := miasmaDarkColors
		if !t.Dark {
			mc = miasmaLightColors
		}
		colors.SelectionFg = hexColor(mc.selectionFg)
		colors.SelectionBg = hexColor(mc.selectionBg)
		colors.DarkerSelectionBg = hexColor(mc.footerBg)
		colors.CurrentBg = hexColor(mc.currentBg)
		colors.MatchFg = hexColor(mc.matchFg)
		colors.MatchBg = hexColor(mc.matchBg)
		colors.MatchFocusedFg = hexColor(mc.matchFocusedFg)
		colors.MatchFocusedBg = hexColor(mc.matchFocusedBg)
		colors.PillFg = hexColor(mc.pillFg)
	}

	return Styles{
		Tint:   t,
		Colors: colors,
	}
}

var SupportedThemeToDeltaSyntax = map[string]string{
	TintMiasmaDark.ID:                 "gruvbox-dark",
	TintMiasmaLight.ID:                "gruvbox-light",
	tint.TintCatppuccinFrappe.ID:      "Catppuccin Frappe",
	tint.TintCatppuccinMacchiato.ID:   "Catppuccin Macchiato",
	tint.TintCatppuccinMocha.ID:       "Catppuccin Mocha",
	tint.TintCatppuccinLatte.ID:       "Catppuccin Latte",
	tint.TintNeon.ID:                  "DarkNeon",
	tint.TintDracula.ID:               "Dracula",
	tint.TintMonokaiPro.ID:            "Monokai Extended",
	tint.TintNord.ID:                  "Nord",
	tint.TintOneHalfDark.ID:           "OneHalfDark",
	tint.TintGruvboxLight.ID:          "gruvbox-light",
	tint.TintGruvboxDark.ID:           "gruvbox-dark",
	tint.TintTokyoNight.ID:            "tokyonight_night",
	tint.TintZenburn.ID:               "zenburn",
	tint.TintGithub.ID:                "GitHub",
	tint.TintDimmedMonokai.ID:         "Monokai Extended Light",
	tint.TintOneHalfLight.ID:          "OneHalfLight",
	tint.TintBuiltinSolarizedLight.ID: "Solarized (light)",
	tint.TintBuiltinSolarizedDark.ID:  "Solarized (dark)",
}

var Themes *tint.Registry

func RegisterSupportedTints() {
	Themes = tint.NewRegistry(
		tint.TintTokyoNight,
		TintMiasmaDark,
		TintMiasmaLight,
		tint.TintCatppuccinFrappe,
		tint.TintCatppuccinMacchiato,
		tint.TintCatppuccinMocha,
		tint.TintCatppuccinLatte,
		tint.TintNeon,
		tint.TintDracula,
		tint.TintMonokaiPro,
		tint.TintNord,
		tint.TintOneHalfDark,
		tint.TintGruvboxLight,
		tint.TintGruvboxDark,
		tint.TintTokyoNight,
		tint.TintZenburn,
		tint.TintGithub,
		tint.TintDimmedMonokai,
		tint.TintOneHalfLight,
		tint.TintBuiltinSolarizedLight,
		tint.TintBuiltinSolarizedDark,
	)
}
