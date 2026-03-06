package controllers

import (
	"github.com/jesseduffield/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/context"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type FileBrowserController struct {
	baseController
	c *ControllerCommon
}

var _ types.IController = &FileBrowserController{}

func NewFileBrowserController(c *ControllerCommon) *FileBrowserController {
	return &FileBrowserController{
		baseController: baseController{},
		c:              c,
	}
}

func (self *FileBrowserController) GetKeybindings(opts types.KeybindingsOpts) []*types.Binding {
	return []*types.Binding{
		{
			Key:         opts.GetKey(opts.Config.Universal.PrevItem),
			Handler:     self.moveUp,
			Description: "Move up",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.NextItem),
			Handler:     self.moveDown,
			Description: "Move down",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.PrevBlock),
			Handler:     self.moveLeft,
			Description: "Previous column",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.NextBlock),
			Handler:     self.moveRight,
			Description: "Next column",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.Select),
			Handler:     self.confirm,
			Description: "Select",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.ConfirmMenu),
			Handler:     self.confirm,
			Description: "Select",
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.Return),
			Handler:     self.close,
			Description: self.c.Tr.Cancel,
		},
		{
			Key:         opts.GetKey(opts.Config.Universal.Edit),
			Handler:     self.edit,
			Description: self.c.Tr.Edit,
		},
	}
}

func (self *FileBrowserController) Context() types.Context {
	return self.c.Contexts().FileBrowser
}

func (self *FileBrowserController) context() *context.FileBrowserContext {
	return self.c.Contexts().FileBrowser
}

func (self *FileBrowserController) moveUp() error {
	self.context().MoveUp()
	return nil
}

func (self *FileBrowserController) moveDown() error {
	self.context().MoveDown()
	return nil
}

func (self *FileBrowserController) moveLeft() error {
	self.context().MoveLeft()
	return nil
}

func (self *FileBrowserController) moveRight() error {
	self.context().MoveRight()
	return nil
}

func (self *FileBrowserController) confirm() error {
	return self.context().Confirm()
}

func (self *FileBrowserController) close() error {
	self.c.Context().Pop()
	return nil
}

func (self *FileBrowserController) edit() error {
	path := self.context().GetSelectedPath()
	if path == "" {
		return nil
	}

	// Close the file browser and edit the file
	self.c.Context().Pop()
	return self.c.Helpers().Files.EditFiles([]string{path})
}

func (self *FileBrowserController) GetMouseKeybindings(opts types.KeybindingsOpts) []*gocui.ViewMouseBinding {
	return []*gocui.ViewMouseBinding{
		{
			ViewName: "fileBrowserLeft",
			Key:      gocui.MouseWheelUp,
			Handler:  func(gocui.ViewMouseBindingOpts) error { return self.scrollLeftUp() },
		},
		{
			ViewName: "fileBrowserLeft",
			Key:      gocui.MouseWheelDown,
			Handler:  func(gocui.ViewMouseBindingOpts) error { return self.scrollLeftDown() },
		},
		{
			ViewName: "fileBrowserMiddle",
			Key:      gocui.MouseWheelUp,
			Handler:  func(gocui.ViewMouseBindingOpts) error { return self.scrollMiddleUp() },
		},
		{
			ViewName: "fileBrowserMiddle",
			Key:      gocui.MouseWheelDown,
			Handler:  func(gocui.ViewMouseBindingOpts) error { return self.scrollMiddleDown() },
		},
		{
			ViewName: "fileBrowserRight",
			Key:      gocui.MouseWheelUp,
			Handler:  func(gocui.ViewMouseBindingOpts) error { return self.scrollRightUp() },
		},
		{
			ViewName: "fileBrowserRight",
			Key:      gocui.MouseWheelDown,
			Handler:  func(gocui.ViewMouseBindingOpts) error { return self.scrollRightDown() },
		},
	}
}

func (self *FileBrowserController) scrollLeftUp() error {
	self.context().ScrollColumn(0, -1)
	return nil
}

func (self *FileBrowserController) scrollLeftDown() error {
	self.context().ScrollColumn(0, 1)
	return nil
}

func (self *FileBrowserController) scrollMiddleUp() error {
	self.context().ScrollColumn(1, -1)
	return nil
}

func (self *FileBrowserController) scrollMiddleDown() error {
	self.context().ScrollColumn(1, 1)
	return nil
}

func (self *FileBrowserController) scrollRightUp() error {
	self.context().ScrollPreview(-3)
	return nil
}

func (self *FileBrowserController) scrollRightDown() error {
	self.context().ScrollPreview(3)
	return nil
}
