package context

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jesseduffield/gocui"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type FileBrowserContext struct {
	c *ContextCommon

	*SimpleContext

	// State
	columns      [3][]FileBrowserEntry
	selectedIdx  [3]int
	paths        [3]string
	activeColumn int // 0 = left, 1 = middle

	// Callbacks
	onFileSelect func(path string) error
}

type FileBrowserEntry struct {
	Name  string
	Path  string
	IsDir bool
}

var _ types.Context = (*FileBrowserContext)(nil)

func NewFileBrowserContext(c *ContextCommon) *FileBrowserContext {
	ctx := &FileBrowserContext{
		c:            c,
		activeColumn: 0,
	}

	ctx.SimpleContext = NewSimpleContext(NewBaseContext(NewBaseContextOpts{
		View:                  c.Views().FileBrowserMiddle,
		WindowName:            "fileBrowser",
		Key:                   FILE_BROWSER_CONTEXT_KEY,
		Kind:                  types.TEMPORARY_POPUP,
		Focusable:             true,
		HasUncontrolledBounds: true,
	}))

	return ctx
}

func (self *FileBrowserContext) SetOnFileSelect(fn func(path string) error) {
	self.onFileSelect = fn
}

func (self *FileBrowserContext) Open(startPath string) error {
	self.paths[0] = filepath.Dir(startPath)
	self.paths[1] = startPath
	self.paths[2] = ""
	self.activeColumn = 1
	self.selectedIdx = [3]int{0, 0, 0}

	// Load parent directory (left column)
	if entries, err := self.loadDirectory(self.paths[0]); err == nil {
		self.columns[0] = entries
		// Find and select the current directory in parent
		for i, e := range entries {
			if e.Path == startPath {
				self.selectedIdx[0] = i
				break
			}
		}
	}

	// Load current directory (middle column)
	if entries, err := self.loadDirectory(startPath); err == nil {
		self.columns[1] = entries
	}

	// Load first item's contents if it's a directory (right column)
	if len(self.columns[1]) > 0 && self.columns[1][0].IsDir {
		if entries, err := self.loadDirectory(self.columns[1][0].Path); err == nil {
			self.columns[2] = entries
			self.paths[2] = self.columns[1][0].Path
		}
	}

	self.render()
	return nil
}

func (self *FileBrowserContext) loadDirectory(dirPath string) ([]FileBrowserEntry, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var dirs []FileBrowserEntry
	var files []FileBrowserEntry

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		e := FileBrowserEntry{
			Name:  entry.Name(),
			Path:  filepath.Join(dirPath, entry.Name()),
			IsDir: entry.IsDir(),
		}
		if entry.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return append(dirs, files...), nil
}

func (self *FileBrowserContext) render() {
	// Render left column
	self.renderColumn(self.c.Views().FileBrowserLeft, self.columns[0], self.selectedIdx[0], self.activeColumn == 0)

	// Render middle column
	self.renderColumn(self.c.Views().FileBrowserMiddle, self.columns[1], self.selectedIdx[1], self.activeColumn == 1)

	// Render right column (preview)
	self.renderPreview()
}

func (self *FileBrowserContext) renderColumn(view *gocui.View, entries []FileBrowserEntry, selectedIdx int, isActive bool) {
	var lines []string
	for _, entry := range entries {
		icon := "📄"
		if entry.IsDir {
			icon = "📁"
		}

		line := icon + " " + entry.Name
		if entry.IsDir {
			line += "/"
		}

		lines = append(lines, line)
	}

	view.SetContent(strings.Join(lines, "\n"))

	if selectedIdx >= 0 && selectedIdx < len(entries) {
		view.SetCursor(0, selectedIdx)
		view.SetOrigin(0, max(0, selectedIdx-view.InnerHeight()/2))
	}

	// Update title with actual directory name
	if len(entries) > 0 {
		path := filepath.Dir(entries[0].Path)
		view.Title = filepath.Base(path)
	}
}

func (self *FileBrowserContext) renderPreview() {
	view := self.c.Views().FileBrowserRight

	// Get selected entry from middle column
	if len(self.columns[1]) == 0 {
		view.Clear()
		return
	}

	entry := self.columns[1][self.selectedIdx[1]]

	if entry.IsDir {
		// Show directory contents
		var lines []string
		for _, e := range self.columns[2] {
			icon := "📄"
			if e.IsDir {
				icon = "📁"
			}
			line := icon + " " + e.Name
			if e.IsDir {
				line += "/"
			}
			lines = append(lines, line)
		}
		view.SetContent(strings.Join(lines, "\n"))
		view.Title = entry.Name
	} else {
		// Show file preview
		self.renderFilePreview(view, entry.Path)
		view.Title = entry.Name
	}
}

func (self *FileBrowserContext) renderFilePreview(view *gocui.View, filePath string) {
	// Get relative path for git operations
	cwd, _ := os.Getwd()
	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil {
		relPath = filePath
	}

	var parts []string

	// Check for git diff
	diff, err := self.c.Git().Diff.DiffCmdObj([]string{"--", relPath}).RunWithOutput()
	if err == nil && strings.TrimSpace(diff) != "" {
		parts = append(parts, "── Diff ──\n"+strings.TrimSpace(diff))
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err == nil {
		lines := strings.Split(string(content), "\n")
		maxLines := 100
		if len(lines) > maxLines {
			lines = lines[:maxLines]
			lines = append(lines, "... (truncated)")
		}
		preview := strings.Join(lines, "\n")
		if preview != "" {
			if len(parts) > 0 {
				parts = append(parts, "\n── Content ──\n"+preview)
			} else {
				parts = append(parts, preview)
			}
		}
	}

	view.SetContent(strings.Join(parts, "\n"))
}

// Navigation methods
func (self *FileBrowserContext) MoveUp() {
	col := self.activeColumn
	if self.selectedIdx[col] > 0 {
		self.selectedIdx[col]--
		self.updateAfterSelection()
	}
}

func (self *FileBrowserContext) MoveDown() {
	col := self.activeColumn
	if self.selectedIdx[col] < len(self.columns[col])-1 {
		self.selectedIdx[col]++
		self.updateAfterSelection()
	}
}

func (self *FileBrowserContext) MoveLeft() {
	if self.activeColumn > 0 {
		self.activeColumn--
		self.render()
	} else {
		// Navigate to parent directory
		self.navigateToParent()
	}
}

func (self *FileBrowserContext) MoveRight() {
	if self.activeColumn < 1 {
		self.activeColumn++
		self.render()
	} else {
		// Navigate into selected directory
		self.navigateInto()
	}
}

func (self *FileBrowserContext) navigateToParent() {
	parentPath := filepath.Dir(self.paths[0])
	if parentPath == self.paths[0] {
		return // Already at root
	}

	// Shift everything right
	self.columns[2] = self.columns[1]
	self.paths[2] = self.paths[1]
	self.selectedIdx[2] = self.selectedIdx[1]

	self.columns[1] = self.columns[0]
	self.paths[1] = self.paths[0]
	self.selectedIdx[1] = self.selectedIdx[0]

	// Load new parent
	self.paths[0] = parentPath
	if entries, err := self.loadDirectory(parentPath); err == nil {
		self.columns[0] = entries
		// Find current dir in parent
		for i, e := range entries {
			if e.Path == self.paths[1] {
				self.selectedIdx[0] = i
				break
			}
		}
	}

	self.render()
}

func (self *FileBrowserContext) navigateInto() {
	if len(self.columns[1]) == 0 {
		return
	}

	entry := self.columns[1][self.selectedIdx[1]]
	if !entry.IsDir {
		return
	}

	// Shift everything left
	self.columns[0] = self.columns[1]
	self.paths[0] = self.paths[1]
	self.selectedIdx[0] = self.selectedIdx[1]

	self.columns[1] = self.columns[2]
	self.paths[1] = entry.Path
	self.selectedIdx[1] = 0

	// Load new right column
	if len(self.columns[1]) > 0 && self.columns[1][0].IsDir {
		if entries, err := self.loadDirectory(self.columns[1][0].Path); err == nil {
			self.columns[2] = entries
			self.paths[2] = self.columns[1][0].Path
		}
	} else {
		self.columns[2] = nil
		self.paths[2] = ""
	}

	self.render()
}

func (self *FileBrowserContext) updateAfterSelection() {
	if self.activeColumn == 0 {
		self.updateAfterLeftSelection()
	} else if self.activeColumn == 1 {
		self.updateAfterMiddleSelection()
	}
	self.render()
}

func (self *FileBrowserContext) Confirm() error {
	if len(self.columns[1]) == 0 {
		return nil
	}

	entry := self.columns[1][self.selectedIdx[1]]
	if entry.IsDir {
		self.navigateInto()
		return nil
	}

	// File selected - call callback
	if self.onFileSelect != nil {
		return self.onFileSelect(entry.Path)
	}
	return nil
}

func (self *FileBrowserContext) GetSelectedPath() string {
	col := self.activeColumn
	if len(self.columns[col]) == 0 {
		return ""
	}
	return self.columns[col][self.selectedIdx[col]].Path
}

func (self *FileBrowserContext) GetSelectedEntry() *FileBrowserEntry {
	col := self.activeColumn
	if len(self.columns[col]) == 0 {
		return nil
	}
	return &self.columns[col][self.selectedIdx[col]]
}

func (self *FileBrowserContext) HandleFocusLost(opts types.OnFocusLostOpts) {
	// Hide the file browser views when losing focus
	self.c.Views().FileBrowserLeft.Visible = false
	self.c.Views().FileBrowserMiddle.Visible = false
	self.c.Views().FileBrowserRight.Visible = false
}

// ScrollColumn scrolls a specific column (0=left, 1=middle) by moving selection
func (self *FileBrowserContext) ScrollColumn(col int, delta int) {
	if col < 0 || col > 1 {
		return
	}

	newIdx := self.selectedIdx[col] + delta
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(self.columns[col]) {
		newIdx = len(self.columns[col]) - 1
	}
	if newIdx < 0 {
		newIdx = 0
	}

	if newIdx != self.selectedIdx[col] {
		self.selectedIdx[col] = newIdx
		// Update dependent columns if needed
		if col == 0 {
			self.updateAfterLeftSelection()
		} else if col == 1 {
			self.updateAfterMiddleSelection()
		}
		self.render()
	}
}

// ScrollPreview scrolls the right preview pane
func (self *FileBrowserContext) ScrollPreview(delta int) {
	view := self.c.Views().FileBrowserRight
	ox, oy := view.Origin()
	newOy := oy + delta
	if newOy < 0 {
		newOy = 0
	}
	view.SetOrigin(ox, newOy)
}

func (self *FileBrowserContext) updateAfterLeftSelection() {
	if len(self.columns[0]) == 0 {
		return
	}
	entry := self.columns[0][self.selectedIdx[0]]
	if entry.IsDir {
		if entries, err := self.loadDirectory(entry.Path); err == nil {
			self.columns[1] = entries
			self.paths[1] = entry.Path
			self.selectedIdx[1] = 0

			if len(entries) > 0 && entries[0].IsDir {
				if entries2, err := self.loadDirectory(entries[0].Path); err == nil {
					self.columns[2] = entries2
					self.paths[2] = entries[0].Path
				}
			} else {
				self.columns[2] = nil
				self.paths[2] = ""
			}
		}
	}
}

func (self *FileBrowserContext) updateAfterMiddleSelection() {
	if len(self.columns[1]) == 0 {
		return
	}
	entry := self.columns[1][self.selectedIdx[1]]
	if entry.IsDir {
		if entries, err := self.loadDirectory(entry.Path); err == nil {
			self.columns[2] = entries
			self.paths[2] = entry.Path
		}
	} else {
		self.columns[2] = nil
		self.paths[2] = ""
	}
}
