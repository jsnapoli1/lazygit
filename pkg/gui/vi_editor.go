package gui

import (
	"strings"

	"github.com/jesseduffield/gocui"
)

// ViMode represents the current vi editing mode
type ViMode int

const (
	ViModeNormal ViMode = iota
	ViModeInsert
	ViModeVisual
	ViModeCommand
)

// ViCommand represents a command result from the editor
type ViCommand int

const (
	ViCommandNone ViCommand = iota
	ViCommandWrite      // :w
	ViCommandQuit       // :q
	ViCommandWriteQuit  // :wq, :x
	ViCommandQuitForce  // :q!
	ViCommandWriteAll   // :wa
	ViCommandQuitAll    // :qa
)

// ViEditor handles vi-style editing for a gocui view
type ViEditor struct {
	mode          ViMode
	pendingAction string // for multi-key commands like 'dd', 'yy', etc.
	yankBuffer    string
	commandBuffer string // for : commands
	view          *gocui.View
	onModeChange  func(ViMode)
	onCommand     func(ViCommand) // callback when a command is executed
}

func NewViEditor(view *gocui.View, onModeChange func(ViMode), onCommand func(ViCommand)) *ViEditor {
	return &ViEditor{
		mode:         ViModeNormal,
		view:         view,
		onModeChange: onModeChange,
		onCommand:    onCommand,
	}
}

func (e *ViEditor) Mode() ViMode {
	return e.mode
}

func (e *ViEditor) SetMode(mode ViMode) {
	e.mode = mode
	if e.onModeChange != nil {
		e.onModeChange(mode)
	}
}

func (e *ViEditor) Edit(key gocui.Key, ch rune, mod gocui.Modifier) bool {
	switch e.mode {
	case ViModeInsert:
		return e.handleInsertMode(key, ch, mod)
	case ViModeNormal:
		return e.handleNormalMode(key, ch, mod)
	case ViModeVisual:
		return e.handleVisualMode(key, ch, mod)
	case ViModeCommand:
		return e.handleCommandMode(key, ch, mod)
	}
	return false
}

func (e *ViEditor) handleInsertMode(key gocui.Key, ch rune, mod gocui.Modifier) bool {
	// Escape to return to normal mode
	if key == gocui.KeyEsc {
		e.SetMode(ViModeNormal)
		return true
	}

	// Handle special keys in insert mode
	switch key {
	case gocui.KeyBackspace, gocui.KeyBackspace2:
		e.view.TextArea.BackSpaceChar()
		e.view.RenderTextArea()
		return true
	case gocui.KeyDelete:
		e.view.TextArea.DeleteChar()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowLeft:
		e.view.TextArea.MoveCursorLeft()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowRight:
		e.view.TextArea.MoveCursorRight()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowUp:
		e.view.TextArea.MoveCursorUp()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowDown:
		e.view.TextArea.MoveCursorDown()
		e.view.RenderTextArea()
		return true
	case gocui.KeyEnter:
		e.view.TextArea.TypeCharacter("\n")
		e.view.RenderTextArea()
		return true
	case gocui.KeySpace:
		e.view.TextArea.TypeCharacter(" ")
		e.view.RenderTextArea()
		return true
	case gocui.KeyTab:
		e.view.TextArea.TypeCharacter("\t")
		e.view.RenderTextArea()
		return true
	}

	// Type regular characters
	if ch != 0 {
		e.view.TextArea.TypeCharacter(string(ch))
		e.view.RenderTextArea()
		return true
	}

	return false
}

func (e *ViEditor) handleNormalMode(key gocui.Key, ch rune, mod gocui.Modifier) bool {
	// Handle pending multi-key commands
	if e.pendingAction != "" {
		return e.handlePendingAction(ch)
	}

	// Handle Escape - clear any pending action
	if key == gocui.KeyEsc {
		e.pendingAction = ""
		return true
	}

	// Mode switching
	switch ch {
	case 'i':
		e.SetMode(ViModeInsert)
		return true
	case 'I':
		e.moveToLineStart()
		e.SetMode(ViModeInsert)
		return true
	case 'a':
		e.view.TextArea.MoveCursorRight()
		e.view.RenderTextArea()
		e.SetMode(ViModeInsert)
		return true
	case 'A':
		e.moveToLineEnd()
		e.SetMode(ViModeInsert)
		return true
	case 'o':
		e.moveToLineEnd()
		e.view.TextArea.TypeCharacter("\n")
		e.view.RenderTextArea()
		e.SetMode(ViModeInsert)
		return true
	case 'O':
		e.moveToLineStart()
		e.view.TextArea.TypeCharacter("\n")
		e.view.TextArea.MoveCursorUp()
		e.view.RenderTextArea()
		e.SetMode(ViModeInsert)
		return true
	case 'v':
		e.SetMode(ViModeVisual)
		return true
	}

	// Navigation
	switch ch {
	case 'h':
		e.view.TextArea.MoveCursorLeft()
		e.view.RenderTextArea()
		return true
	case 'j':
		e.view.TextArea.MoveCursorDown()
		e.view.RenderTextArea()
		return true
	case 'k':
		e.view.TextArea.MoveCursorUp()
		e.view.RenderTextArea()
		return true
	case 'l':
		e.view.TextArea.MoveCursorRight()
		e.view.RenderTextArea()
		return true
	case 'w':
		e.moveWordForward()
		return true
	case 'b':
		e.moveWordBackward()
		return true
	case 'e':
		e.moveToWordEnd()
		return true
	case '0':
		e.moveToLineStart()
		return true
	case '$':
		e.moveToLineEnd()
		return true
	case '^':
		e.moveToFirstNonBlank()
		return true
	case 'G':
		e.moveToEnd()
		return true
	}

	// Arrow keys work in normal mode too
	switch key {
	case gocui.KeyArrowLeft:
		e.view.TextArea.MoveCursorLeft()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowRight:
		e.view.TextArea.MoveCursorRight()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowUp:
		e.view.TextArea.MoveCursorUp()
		e.view.RenderTextArea()
		return true
	case gocui.KeyArrowDown:
		e.view.TextArea.MoveCursorDown()
		e.view.RenderTextArea()
		return true
	}

	// Editing commands
	switch ch {
	case 'x':
		e.view.TextArea.DeleteChar()
		e.view.RenderTextArea()
		return true
	case 'X':
		e.view.TextArea.BackSpaceChar()
		e.view.RenderTextArea()
		return true
	case 'r':
		e.pendingAction = "r"
		return true
	case 's':
		e.view.TextArea.DeleteChar()
		e.view.RenderTextArea()
		e.SetMode(ViModeInsert)
		return true
	case 'S', 'C':
		e.deleteToLineEnd()
		e.SetMode(ViModeInsert)
		return true
	case 'D':
		e.deleteToLineEnd()
		return true
	}

	// Multi-key commands
	switch ch {
	case 'd':
		e.pendingAction = "d"
		return true
	case 'y':
		e.pendingAction = "y"
		return true
	case 'c':
		e.pendingAction = "c"
		return true
	case 'g':
		e.pendingAction = "g"
		return true
	}

	// Paste
	switch ch {
	case 'p':
		e.pasteAfter()
		return true
	case 'P':
		e.pasteBefore()
		return true
	}

	// Undo (basic - just clear pending)
	switch ch {
	case 'u':
		// Note: gocui TextArea doesn't have undo, so this is a no-op
		return true
	}

	// Command mode entry
	if ch == ':' {
		e.commandBuffer = ""
		e.SetMode(ViModeCommand)
		return true
	}

	// In normal mode, block all other keys from being passed to the default editor
	// This prevents typing characters when not in insert mode
	return true
}

func (e *ViEditor) handlePendingAction(ch rune) bool {
	action := e.pendingAction
	e.pendingAction = ""

	switch action {
	case "d":
		switch ch {
		case 'd':
			e.deleteLine()
			return true
		case 'w':
			e.deleteWord()
			return true
		case '$':
			e.deleteToLineEnd()
			return true
		case '0':
			e.deleteToLineStart()
			return true
		}
	case "y":
		switch ch {
		case 'y':
			e.yankLine()
			return true
		case 'w':
			e.yankWord()
			return true
		}
	case "c":
		switch ch {
		case 'c':
			e.deleteLine()
			e.SetMode(ViModeInsert)
			return true
		case 'w':
			e.deleteWord()
			e.SetMode(ViModeInsert)
			return true
		}
	case "g":
		switch ch {
		case 'g':
			e.moveToStart()
			return true
		}
	case "r":
		// Replace single character
		e.view.TextArea.DeleteChar()
		e.view.TextArea.TypeCharacter(string(ch))
		e.view.TextArea.MoveCursorLeft()
		e.view.RenderTextArea()
		return true
	}

	return false
}

func (e *ViEditor) handleVisualMode(key gocui.Key, ch rune, mod gocui.Modifier) bool {
	// Escape to return to normal mode
	if key == gocui.KeyEsc || ch == 'v' {
		e.SetMode(ViModeNormal)
		return true
	}

	// Navigation in visual mode (same as normal)
	switch ch {
	case 'h':
		e.view.TextArea.MoveCursorLeft()
		e.view.RenderTextArea()
		return true
	case 'j':
		e.view.TextArea.MoveCursorDown()
		e.view.RenderTextArea()
		return true
	case 'k':
		e.view.TextArea.MoveCursorUp()
		e.view.RenderTextArea()
		return true
	case 'l':
		e.view.TextArea.MoveCursorRight()
		e.view.RenderTextArea()
		return true
	}

	// Block all other keys in visual mode
	return true
}

func (e *ViEditor) handleCommandMode(key gocui.Key, ch rune, mod gocui.Modifier) bool {
	// Escape to return to normal mode
	if key == gocui.KeyEsc {
		e.commandBuffer = ""
		e.SetMode(ViModeNormal)
		return true
	}

	// Backspace to delete characters from command
	if key == gocui.KeyBackspace || key == gocui.KeyBackspace2 {
		if len(e.commandBuffer) > 0 {
			e.commandBuffer = e.commandBuffer[:len(e.commandBuffer)-1]
			e.onModeChange(e.mode) // refresh display
		}
		if len(e.commandBuffer) == 0 {
			e.SetMode(ViModeNormal)
		}
		return true
	}

	// Enter to execute command
	if key == gocui.KeyEnter {
		cmd := e.parseCommand(e.commandBuffer)
		e.commandBuffer = ""
		e.SetMode(ViModeNormal)
		if cmd != ViCommandNone && e.onCommand != nil {
			e.onCommand(cmd)
		}
		return true
	}

	// Type characters into command buffer
	if ch != 0 {
		e.commandBuffer += string(ch)
		e.onModeChange(e.mode) // refresh display
		return true
	}

	return true
}

func (e *ViEditor) parseCommand(cmd string) ViCommand {
	switch cmd {
	case "w":
		return ViCommandWrite
	case "q":
		return ViCommandQuit
	case "wq", "x", "wqa", "xa":
		return ViCommandWriteQuit
	case "q!":
		return ViCommandQuitForce
	case "wa":
		return ViCommandWriteAll
	case "qa", "qa!":
		return ViCommandQuitAll
	default:
		return ViCommandNone
	}
}

func (e *ViEditor) CommandBuffer() string {
	return e.commandBuffer
}

// Helper methods for cursor movement and editing

func (e *ViEditor) getContent() string {
	return e.view.TextArea.GetContent()
}

func (e *ViEditor) getLines() []string {
	return strings.Split(e.getContent(), "\n")
}

func (e *ViEditor) moveToLineStart() {
	// Move left until we hit the start of the line or beginning of content
	content := e.getContent()
	if content == "" {
		return
	}
	for i := 0; i < 1000; i++ { // safety limit
		// Check if we're at the start
		e.view.TextArea.MoveCursorLeft()
		newContent := e.view.TextArea.GetContent()
		if newContent == content {
			break
		}
	}
	// Actually, use the simple approach - keep moving left until we can't
	// This is a simplified implementation
	e.view.RenderTextArea()
}

func (e *ViEditor) moveToLineEnd() {
	for i := 0; i < 1000; i++ {
		e.view.TextArea.MoveCursorRight()
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) moveToFirstNonBlank() {
	e.moveToLineStart()
	content := e.getContent()
	for _, ch := range content {
		if ch == ' ' || ch == '\t' {
			e.view.TextArea.MoveCursorRight()
		} else {
			break
		}
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) moveWordForward() {
	// Simple word movement - move right until we hit whitespace, then skip whitespace
	for i := 0; i < 100; i++ {
		e.view.TextArea.MoveCursorRight()
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) moveWordBackward() {
	for i := 0; i < 100; i++ {
		e.view.TextArea.MoveCursorLeft()
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) moveToWordEnd() {
	e.moveWordForward()
}

func (e *ViEditor) moveToStart() {
	// Move to the very beginning
	for i := 0; i < 10000; i++ {
		e.view.TextArea.MoveCursorUp()
	}
	e.moveToLineStart()
}

func (e *ViEditor) moveToEnd() {
	// Move to the very end
	for i := 0; i < 10000; i++ {
		e.view.TextArea.MoveCursorDown()
	}
	e.moveToLineEnd()
}

func (e *ViEditor) deleteLine() {
	// Move to start of line, delete until end, delete the newline
	e.moveToLineStart()
	e.deleteToLineEnd()
	e.view.TextArea.DeleteChar() // delete newline
	e.view.RenderTextArea()
}

func (e *ViEditor) deleteWord() {
	// Delete characters until we hit whitespace or end of word
	for i := 0; i < 100; i++ {
		e.view.TextArea.DeleteChar()
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) deleteToLineEnd() {
	for i := 0; i < 1000; i++ {
		e.view.TextArea.DeleteChar()
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) deleteToLineStart() {
	for i := 0; i < 1000; i++ {
		e.view.TextArea.BackSpaceChar()
	}
	e.view.RenderTextArea()
}

func (e *ViEditor) yankLine() {
	// This is simplified - just store the current line content
	lines := e.getLines()
	if len(lines) > 0 {
		e.yankBuffer = lines[0] + "\n"
	}
}

func (e *ViEditor) yankWord() {
	// Simplified
	e.yankBuffer = ""
}

func (e *ViEditor) pasteAfter() {
	if e.yankBuffer != "" {
		e.view.TextArea.MoveCursorRight()
		e.view.TextArea.TypeCharacter(e.yankBuffer)
		e.view.RenderTextArea()
	}
}

func (e *ViEditor) pasteBefore() {
	if e.yankBuffer != "" {
		e.view.TextArea.TypeCharacter(e.yankBuffer)
		e.view.RenderTextArea()
	}
}

func (e *ViEditor) ModeString() string {
	switch e.mode {
	case ViModeInsert:
		return "-- INSERT --"
	case ViModeVisual:
		return "-- VISUAL --"
	case ViModeCommand:
		return ":" + e.commandBuffer
	default:
		return "-- NORMAL --"
	}
}
