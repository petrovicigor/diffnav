package common

import (
	"image/color"
	"os"

	tint "github.com/lrstanley/bubbletint/v2"
)

// MiasmaThemeID is the theme name that picks the Miasma dark or light tint
// automatically (see ResolveThemeID). It is not a registered tint itself.
const MiasmaThemeID = "miasma"

// The Miasma palettes (dark + the custom parchment light), taken from the
// kitty/nvim/tmux Miasma themes so diffnav matches the rest of the desktop.
var (
	TintMiasmaDark = &tint.Tint{
		DisplayName:  "Miasma Dark",
		ID:           "miasma_dark",
		Dark:         true,
		Fg:           tint.FromHex("#c2c2b0"),
		Bg:           tint.FromHex("#222222"),
		SelectionBg:  tint.FromHex("#2e2a24"),
		Cursor:       tint.FromHex("#c7c7c7"),
		Black:        tint.FromHex("#555544"), // borders, dim text (nvim FloatBorder)
		BrightBlack:  tint.FromHex("#666666"), // comments / separators
		Blue:         tint.FromHex("#78824b"), // olive accent
		BrightBlue:   tint.FromHex("#78824b"), // directories (nvim Directory)
		Cyan:         tint.FromHex("#c9a554"),
		BrightCyan:   tint.FromHex("#d7c483"),
		Green:        tint.FromHex("#5f875f"),
		BrightGreen:  tint.FromHex("#5f875f"),
		Red:          tint.FromHex("#b36d43"),
		BrightRed:    tint.FromHex("#b36d43"),
		Yellow:       tint.FromHex("#c9a554"),
		BrightYellow: tint.FromHex("#c9a554"),
		Purple:       tint.FromHex("#bb7744"),
		BrightPurple: tint.FromHex("#bb7744"),
		White:        tint.FromHex("#c2c2b0"),
		BrightWhite:  tint.FromHex("#d7c483"),
	}

	TintMiasmaLight = &tint.Tint{
		DisplayName:  "Miasma Light",
		ID:           "miasma_light",
		Dark:         false,
		Fg:           tint.FromHex("#433a2a"),
		Bg:           tint.FromHex("#e8e0c8"),
		SelectionBg:  tint.FromHex("#dad0b4"),
		Cursor:       tint.FromHex("#6b6245"),
		Black:        tint.FromHex("#a09880"),
		BrightBlack:  tint.FromHex("#8a8070"),
		Blue:         tint.FromHex("#5c6838"),
		BrightBlue:   tint.FromHex("#5c6838"),
		Cyan:         tint.FromHex("#7d6b2e"),
		BrightCyan:   tint.FromHex("#6b5520"),
		Green:        tint.FromHex("#4a6a4a"),
		BrightGreen:  tint.FromHex("#4a6a4a"),
		Red:          tint.FromHex("#8b5a2b"),
		BrightRed:    tint.FromHex("#8b5a2b"),
		Yellow:       tint.FromHex("#7d6b2e"),
		BrightYellow: tint.FromHex("#7d6b2e"),
		Purple:       tint.FromHex("#925e2e"),
		BrightPurple: tint.FromHex("#925e2e"),
		White:        tint.FromHex("#6b6245"),
		BrightWhite:  tint.FromHex("#433a2a"),
	}
)

// miasmaColors are the UI colours Miasma sets by hand instead of deriving
// them from the tint (the derivations darken Blue, which is wrong on a light
// background).
type miasmaColors struct {
	selectionFg, selectionBg, footerBg, currentBg    string
	matchFg, matchBg, matchFocusedFg, matchFocusedBg string
	// pillFg is the scroll pill's text on its BrightBlack body: the tint's
	// White is a dark brown in light mode, near-invisible there (1.6:1).
	pillFg string
}

var (
	miasmaDarkColors = miasmaColors{
		selectionFg:    "#78824b",
		selectionBg:    "#2e2a24", // claude-sessions picker selected row
		footerBg:       "#1c1c1c",
		currentBg:      "#2f3424", // olive wash: the file in view
		matchFg:        "#222222",
		matchBg:        "#5f875f", // nvim Search
		matchFocusedFg: "#222222",
		matchFocusedBg: "#bb7744", // nvim IncSearch
		pillFg:         "#c2c2b0", // White, as before (3.1:1 on #666666)
	}
	miasmaLightColors = miasmaColors{
		selectionFg:    "#5c6838",
		selectionBg:    "#dad0b4",
		footerBg:       "#ddd5b8",
		currentBg:      "#d3d4b0",
		matchFg:        "#e8e0c8",
		matchBg:        "#7d6b2e",
		matchFocusedFg: "#e8e0c8",
		matchFocusedBg: "#925e2e",
		pillFg:         "#e8e0c8", // parchment on #8a8070 (3.0:1), dark mode's mirror
	}
)

// IsMiasma reports whether t is one of the Miasma tints.
func IsMiasma(t *tint.Tint) bool {
	return t != nil && (t.ID == TintMiasmaDark.ID || t.ID == TintMiasmaLight.ID)
}

// MiasmaThemeForMode maps a mode word to the Miasma tint ID: "light" picks
// the light tint, anything else the dark one.
func MiasmaThemeForMode(mode string) string {
	if mode == "light" {
		return TintMiasmaLight.ID
	}
	return TintMiasmaDark.ID
}

// ResolveThemeID turns the configured theme into a registered tint ID. The
// "miasma" theme follows MIASMA_MODE when it is set ("light" / "dark") and is
// dark otherwise; auto reports whether the mode was left open, so the caller
// should ask the terminal for its background and switch if it is light.
func ResolveThemeID(id string, getenv func(string) string) (resolved string, auto bool) {
	if id != MiasmaThemeID {
		return id, false
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	switch mode := getenv("MIASMA_MODE"); mode {
	case "light", "dark":
		return MiasmaThemeForMode(mode), false
	}
	return TintMiasmaDark.ID, true
}

func hexColor(s string) color.Color {
	return tint.FromHex(s)
}
