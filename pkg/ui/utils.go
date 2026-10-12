package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/dlvhdr/diffnav/pkg/constants"
	"github.com/dlvhdr/diffnav/pkg/filenode"
)

func sortFiles(files []*gitdiff.File) {
	slices.SortFunc(files, func(a *gitdiff.File, b *gitdiff.File) int {
		nameA := filenode.GetFileName(a)
		nameB := filenode.GetFileName(b)
		dira := filepath.Dir(nameA)
		dirb := filepath.Dir(nameB)
		if dira != constants.RootName && dirb != constants.RootName && dira == dirb {
			return strings.Compare(strings.ToLower(nameA), strings.ToLower(nameB))
		}

		if dira != constants.RootName && dirb == constants.RootName {
			return -1
		}
		if dirb != constants.RootName && dira == constants.RootName {
			return 1
		}

		if dira != constants.RootName && dirb != constants.RootName {
			if strings.HasPrefix(dira, dirb) {
				return -1
			}

			if strings.HasPrefix(dirb, dira) {
				return 1
			}
		}

		return strings.Compare(strings.ToLower(nameA), strings.ToLower(nameB))
	})
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / 24 / 30)
		return fmt.Sprintf("%dmo", months)
	default:
		years := int(d.Hours() / 24 / 365)
		return fmt.Sprintf("%dy", years)
	}
}

// teeAt turns the separator ending row of block (a pane whose right border
// is its last cell) into "├", so the rule the diff viewer draws on that row
// joins the separator instead of butting into it. Out-of-range rows and
// rows without a border are left alone.
func teeAt(block string, row int) string {
	if row < 0 {
		return block
	}
	lines := strings.Split(block, "\n")
	if row >= len(lines) {
		return block
	}
	i := strings.LastIndex(lines[row], "│")
	if i < 0 {
		return block
	}
	lines[row] = lines[row][:i] + "├" + lines[row][i+len("│"):]
	return strings.Join(lines, "\n")
}
