package helpers

import (
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type FileEditorHelper struct {
	c *HelperCommon
}

func NewFileEditorHelper(c *HelperCommon) *FileEditorHelper {
	return &FileEditorHelper{
		c: c,
	}
}

func (self *FileEditorHelper) OpenFileEditor(filePath string) error {
	// Show the editor view
	self.showEditorView()

	// Open the file in the editor context
	if err := self.c.Contexts().FileEditor.OpenFile(filePath); err != nil {
		return err
	}

	// Push the editor context
	self.c.Context().Push(self.c.Contexts().FileEditor, types.OnFocusOpts{})
	return nil
}

func (self *FileEditorHelper) showEditorView() {
	// Calculate dimensions for the floating editor
	width, height := self.c.GocuiGui().Size()

	// Leave margin around the popup
	margin := 2
	popupWidth := width - margin*2
	popupHeight := height - margin*2

	x0 := margin
	y0 := margin
	x1 := x0 + popupWidth
	y1 := y0 + popupHeight

	view := self.c.Views().FileEditor
	view.Visible = true
	view.Frame = true
	_, _ = self.c.GocuiGui().SetView(view.Name(), x0, y0, x1, y1, 0)
}
