package helpers

import (
	"os"

	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type FileBrowserHelper struct {
	c           *HelperCommon
	filesHelper *FilesHelper
}

func NewFileBrowserHelper(c *HelperCommon, filesHelper *FilesHelper) *FileBrowserHelper {
	return &FileBrowserHelper{
		c:           c,
		filesHelper: filesHelper,
	}
}

func (self *FileBrowserHelper) OpenFileBrowser() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Set up the file selection callback
	self.c.Contexts().FileBrowser.SetOnFileSelect(func(path string) error {
		// Close the file browser first
		self.c.Context().Pop()
		// Then open the file in editor
		return self.filesHelper.EditFiles([]string{path})
	})

	// Show the file browser views
	self.showFileBrowserViews()

	// Open the file browser at the current directory
	if err := self.c.Contexts().FileBrowser.Open(cwd); err != nil {
		return err
	}

	// Push the file browser context
	self.c.Context().Push(self.c.Contexts().FileBrowser, types.OnFocusOpts{})
	return nil
}

func (self *FileBrowserHelper) showFileBrowserViews() {
	// Calculate dimensions for the three-column layout
	width, height := self.c.GocuiGui().Size()

	// Leave margin around the popup
	margin := 2
	popupWidth := width - margin*2
	popupHeight := height - margin*2

	// Each column gets roughly 1/3 of the width
	colWidth := popupWidth / 3

	x0 := margin
	y0 := margin
	y1 := y0 + popupHeight

	// Left column
	leftView := self.c.Views().FileBrowserLeft
	leftView.Visible = true
	leftView.Frame = true
	leftView.Title = "Parent"
	_, _ = self.c.GocuiGui().SetView(leftView.Name(), x0, y0, x0+colWidth, y1, 0)

	// Middle column (active)
	middleView := self.c.Views().FileBrowserMiddle
	middleView.Visible = true
	middleView.Frame = true
	middleView.Title = "Current"
	_, _ = self.c.GocuiGui().SetView(middleView.Name(), x0+colWidth, y0, x0+colWidth*2, y1, 0)

	// Right column (preview)
	rightView := self.c.Views().FileBrowserRight
	rightView.Visible = true
	rightView.Frame = true
	rightView.Title = "Preview"
	_, _ = self.c.GocuiGui().SetView(rightView.Name(), x0+colWidth*2, y0, x0+popupWidth, y1, 0)
}

func (self *FileBrowserHelper) HideFileBrowserViews() {
	self.c.Views().FileBrowserLeft.Visible = false
	self.c.Views().FileBrowserMiddle.Visible = false
	self.c.Views().FileBrowserRight.Visible = false
}
