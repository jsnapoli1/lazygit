package controllers

import (
	"github.com/jesseduffield/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/context"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type FileEditorController struct {
	baseController
	c *ControllerCommon
}

var _ types.IController = &FileEditorController{}

func NewFileEditorController(c *ControllerCommon) *FileEditorController {
	return &FileEditorController{
		baseController: baseController{},
		c:              c,
	}
}

func (self *FileEditorController) GetKeybindings(opts types.KeybindingsOpts) []*types.Binding {
	// Use Ctrl+S to save and Ctrl+Q to quit (don't intercept Esc/Enter which vi needs)
	return []*types.Binding{
		{
			Key:         gocui.KeyCtrlQ,
			Handler:     self.close,
			Description: "Close without saving",
		},
		{
			Key:         gocui.KeyCtrlS,
			Handler:     self.save,
			Description: "Save and close",
		},
		{
			Key:         gocui.KeyCtrlW,
			Handler:     self.saveOnly,
			Description: "Save",
		},
	}
}

func (self *FileEditorController) Context() types.Context {
	return self.c.Contexts().FileEditor
}

func (self *FileEditorController) context() *context.FileEditorContext {
	return self.c.Contexts().FileEditor
}

func (self *FileEditorController) close() error {
	self.c.Context().Pop()
	return nil
}

func (self *FileEditorController) save() error {
	if err := self.context().SaveFile(); err != nil {
		return err
	}
	self.c.Context().Pop()
	self.c.Toast("File saved")
	return nil
}

func (self *FileEditorController) saveOnly() error {
	if err := self.context().SaveFile(); err != nil {
		return err
	}
	self.c.Toast("File saved")
	return nil
}
