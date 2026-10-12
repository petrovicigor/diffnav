package ui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	tint "github.com/lrstanley/bubbletint/v2"

	"github.com/dlvhdr/diffnav/pkg/ui/common"
	themepicker "github.com/dlvhdr/diffnav/pkg/ui/theme_picker"
)

var (
	lightBg = tea.BackgroundColorMsg{Color: color.RGBA{0xf5, 0xf0, 0xe1, 0xff}}
	darkBg  = tea.BackgroundColorMsg{Color: color.RGBA{0x22, 0x22, 0x22, 0xff}}
)

func autoMiasmaModel(t *testing.T) mainModel {
	t.Helper()
	t.Setenv("MIASMA_MODE", "")
	t.Setenv("DIFFNAV_THEME", "")
	m := newTestMainModel(t)
	t.Cleanup(func() { common.Themes.SetTintID(tint.TintTokyoNight.ID) })
	if !m.autoMiasma {
		t.Fatal("precondition: Miasma with no MIASMA_MODE follows the terminal")
	}
	return m
}

func TestBackgroundReplyPicksMiasmaMode(t *testing.T) {
	m := autoMiasmaModel(t)
	updateMainModel(t, m, lightBg)
	if got := common.Themes.Current().ID; got != common.TintMiasmaLight.ID {
		t.Fatalf("a light terminal should pick Miasma light, got %s", got)
	}
}

// A late terminal reply must not override a theme picked in the picker.
func TestBackgroundReplyAfterPickIsIgnored(t *testing.T) {
	for name, msg := range map[string]tea.Msg{
		"chosen":  themepicker.ThemeChosenMsg{Theme: tint.TintTokyoNight},
		"preview": themepicker.ThemePreviewMsg{Theme: tint.TintTokyoNight},
	} {
		m := autoMiasmaModel(t)
		m = updateMainModel(t, m, msg)
		updateMainModel(t, m, lightBg)
		if got := common.Themes.Current().ID; got != tint.TintTokyoNight.ID {
			t.Errorf("%s: the late reply replaced the picked theme with %s", name, got)
		}
	}
	m := autoMiasmaModel(t)
	m.themePickerOpen = true
	updateMainModel(t, m, darkBg)
	updateMainModel(t, m, lightBg)
	if got := common.Themes.Current().ID; got != common.TintMiasmaDark.ID {
		t.Errorf("a reply while the picker is open changed the theme to %s", got)
	}
}
