package controllers

import (
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
	return []*types.Binding{
		{
			Key:         opts.GetKey(opts.Config.Universal.Return),
			Handler:     self.close,
			Description: self.c.Tr.Cancel,
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.Confirm),
			Handler:     self.save,
			Description: "Save",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.ConfirmMenu),
			Handler:     self.save,
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
