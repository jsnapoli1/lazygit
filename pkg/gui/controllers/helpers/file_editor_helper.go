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
	// Open the file in the editor context
	if err := self.c.Contexts().FileEditor.OpenFile(filePath); err != nil {
		return err
	}

	// Resize and show the editor view
	self.ResizeEditorView()

	// Push the editor context
	self.c.Context().Push(self.c.Contexts().FileEditor, types.OnFocusOpts{})
	return nil
}

func (self *FileEditorHelper) ResizeEditorView() {
	// Calculate dimensions for the floating editor as a centered popup
	width, height := self.c.GocuiGui().Size()

	// Use 80% of the screen for the editor
	panelWidth := width * 4 / 5
	panelHeight := height * 4 / 5

	// Center the popup
	x0 := (width - panelWidth) / 2
	y0 := (height - panelHeight) / 2
	x1 := x0 + panelWidth
	y1 := y0 + panelHeight

	view := self.c.Views().FileEditor
	view.Visible = true
	view.Frame = true
	_, _ = self.c.GocuiGui().SetView(view.Name(), x0, y0, x1, y1, 0)
	_, _ = self.c.GocuiGui().SetViewOnTop(view.Name())
}