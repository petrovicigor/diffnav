package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/dlvhdr/diffnav/pkg/config"
)

// followModel is the program's model: mainModel, plus after every update
// the file tree's "current" mark pointed at the file the diff viewer is
// scrolled to (anything — keys, mouse, a re-render, a resize — can move
// it). A wrapper rather than an edit to mainModel.Update, so rebasing onto
// upstream doesn't conflict there.
type followModel struct {
	mainModel
}

// NewFollowing is New wrapped in followModel: the model to run.
func NewFollowing(opts ModelOpts, cfg config.Config) tea.Model {
	return followModel{New(opts, cfg)}
}

func (f followModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := f.mainModel.Update(msg)
	m, ok := next.(mainModel)
	if !ok {
		return next, cmd
	}
	m.fileTree.SetCurrentFile(m.diffViewer.CurrentFilePath())
	return followModel{m}, cmd
}
