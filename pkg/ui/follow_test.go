package ui

import (
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The program's model marks, in the tree, the file the directory view
// shows once its render lands.
func TestFollowModelMarksCurrentFile(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed")
	}
	m := newTestMainModel(t)
	var model tea.Model = followModel{m}
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	f := model.(followModel)
	var cmd tea.Cmd
	f.diffViewer, cmd = f.diffViewer.SetDirPatch("/", f.files)
	if cmd == nil {
		t.Fatal("expected a render")
	}
	model, _ = f.Update(cmd())
	f = model.(followModel)
	if got, want := f.fileTree.CurrentFile(), "graphql-server/tests/package.json"; got != want {
		t.Fatalf("tree marks %q, want %q", got, want)
	}
}
