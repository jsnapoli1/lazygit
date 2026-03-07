package context

import (
	"os"

	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type FileEditorContext struct {
	c *ContextCommon

	*SimpleContext

	// The file being edited
	filePath string
}

var _ types.Context = (*FileEditorContext)(nil)

func NewFileEditorContext(c *ContextCommon) *FileEditorContext {
	ctx := &FileEditorContext{
		c: c,
	}

	ctx.SimpleContext = NewSimpleContext(NewBaseContext(NewBaseContextOpts{
		View:                  c.Views().FileEditor,
		WindowName:            "fileEditor",
		Key:                   FILE_EDITOR_CONTEXT_KEY,
		Kind:                  types.TEMPORARY_POPUP,
		Focusable:             true,
		HasUncontrolledBounds: true,
	}))

	return ctx
}

func (self *FileEditorContext) OpenFile(filePath string) error {
	self.filePath = filePath

	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Set up the view
	view := self.c.Views().FileEditor
	view.Title = filePath
	view.ClearTextArea()
	view.TextArea.TypeString(string(content))
	view.RenderTextArea()

	return nil
}

func (self *FileEditorContext) SaveFile() error {
	if self.filePath == "" {
		return nil
	}

	content := self.c.Views().FileEditor.TextArea.GetContent()
	return os.WriteFile(self.filePath, []byte(content), 0644)
}

func (self *FileEditorContext) GetFilePath() string {
	return self.filePath
}

func (self *FileEditorContext) HandleFocusLost(opts types.OnFocusLostOpts) {
	self.c.Views().FileEditor.Visible = false
	self.filePath = ""
}
