package patch

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/generics/set"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
	"github.com/jesseduffield/lazygit/pkg/theme"
	"github.com/samber/lo"
)

type DisplayMode int

const (
	// DisplayModeSuccinct shows only snippets around changes (default git diff behavior with line numbers)
	DisplayModeSuccinct DisplayMode = iota
	// DisplayModeClean shows the full file content with line numbers and change highlighting
	DisplayModeClean
)

type patchPresenter struct {
	patch *Patch
	// if true, all following fields are ignored
	plain bool

	// line indices for tagged lines (e.g. lines added to a custom patch)
	incLineIndices *set.Set[int]
}

// formats the patch as a plain string
func formatPlain(patch *Patch) string {
	presenter := &patchPresenter{
		patch:          patch,
		plain:          true,
		incLineIndices: set.New[int](),
	}
	return presenter.format()
}

func formatRangePlain(patch *Patch, startIdx int, endIdx int) string {
	lines := patch.Lines()[startIdx : endIdx+1]
	return strings.Join(
		lo.Map(lines, func(line *PatchLine, _ int) string {
			return line.Content + "\n"
		}),
		"",
	)
}

type FormatViewOpts struct {
	// line indices for tagged lines (e.g. lines added to a custom patch)
	IncLineIndices *set.Set[int]
	// display mode for the patch view
	DisplayMode DisplayMode
	// file content for full file mode (required when DisplayMode is DisplayModeClean)
	FileContent string
}

// formats the patch for rendering within a view, meaning it's coloured and
// highlights selected items
func formatView(patch *Patch, opts FormatViewOpts) string {
	includedLineIndices := opts.IncLineIndices
	if includedLineIndices == nil {
		includedLineIndices = set.New[int]()
	}

	if opts.DisplayMode == DisplayModeSuccinct {
		return formatViewSuccinct(patch, includedLineIndices)
	}

	// DisplayModeClean - full file view with change highlighting
	return formatViewFullFile(patch, opts.FileContent, includedLineIndices)
}

// formatViewSuccinct formats the patch in succinct display mode:
// - Skips header lines (diff --git, index, ---, +++)
// - Skips hunk header lines (@@ ... @@)
// - Shows line numbers on the left
// - Applies color coding (green for additions, red for deletions)
// - Adds blank lines between hunks to show section breaks
func formatViewSuccinct(patch *Patch, incLineIndices *set.Set[int]) string {
	if !patch.ContainsChanges() {
		return ""
	}

	stringBuilder := &strings.Builder{}
	lineIdx := len(patch.header) // Start counting from after header

	// Find the maximum line number for formatting width
	maxLineNum := 0
	for _, hunk := range patch.hunks {
		endLineNum := hunk.newStart + hunk.newLength() - 1
		if endLineNum > maxLineNum {
			maxLineNum = endLineNum
		}
	}
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))
	if lineNumWidth < 3 {
		lineNumWidth = 3
	}

	for hunkIdx, hunk := range patch.hunks {
		// Add blank line separator between hunks (not before the first one)
		if hunkIdx > 0 {
			stringBuilder.WriteString("\n")
		}

		lineIdx++ // Skip hunk header line
		currentLineNum := hunk.newStart

		for _, line := range hunk.bodyLines {
			lineStyle := patchLineStyle(line)
			lineNumStr := ""
			content := line.Content

			switch line.Kind {
			case ADDITION:
				lineNumStr = fmt.Sprintf("%*d", lineNumWidth, currentLineNum)
				currentLineNum++
			case DELETION:
				// Deletions don't have a line number in the new file
				lineNumStr = strings.Repeat(" ", lineNumWidth)
			case CONTEXT:
				lineNumStr = fmt.Sprintf("%*d", lineNumWidth, currentLineNum)
				currentLineNum++
			case NEWLINE_MESSAGE:
				lineNumStr = strings.Repeat(" ", lineNumWidth)
			}

			// Format: "  42 │ +added line"
			prefix := fmt.Sprintf("%s │ ", lineNumStr)

			included := line.IsChange() && incLineIndices.Includes(lineIdx)

			// Build the formatted line
			formattedLine := formatSuccinctLine(prefix, content, lineStyle, included)
			stringBuilder.WriteString(formattedLine + "\n")

			lineIdx++
		}
	}

	return stringBuilder.String()
}

func formatSuccinctLine(prefix string, content string, textStyle style.TextStyle, included bool) string {
	prefixStyle := theme.DefaultTextColor
	firstCharStyle := textStyle
	if included {
		firstCharStyle = firstCharStyle.MergeStyle(style.BgGreen)
	}

	// The content starts with +/- or space, we want to color that specially
	if len(content) == 0 {
		return prefixStyle.Sprint(prefix)
	}

	if len(content) == 1 {
		return prefixStyle.Sprint(prefix) + firstCharStyle.Sprint(content)
	}

	return prefixStyle.Sprint(prefix) + firstCharStyle.Sprint(content[:1]) + textStyle.Sprint(content[1:])
}

// formatViewFullFile formats the patch showing the full file content with changes highlighted.
// It merges the file content with the diff to show:
// - Regular lines in default color
// - Added lines highlighted in green
// - Deleted lines inserted at their original positions in red
func formatViewFullFile(patch *Patch, fileContent string, incLineIndices *set.Set[int]) string {
	if fileContent == "" {
		// Fall back to succinct mode if no file content provided
		return formatViewSuccinct(patch, incLineIndices)
	}

	// Split file content into lines
	fileLines := strings.Split(fileContent, "\n")
	// Remove trailing empty line if file ends with newline
	if len(fileLines) > 0 && fileLines[len(fileLines)-1] == "" {
		fileLines = fileLines[:len(fileLines)-1]
	}

	// Build a map of line changes from the patch
	// Key: line number in new file (1-based), Value: slice of deleted lines to show before this line
	deletedLinesBeforeLine := make(map[int][]string)
	addedLines := make(map[int]bool) // Lines that are additions (1-based)

	for _, hunk := range patch.hunks {
		newLineNum := hunk.newStart
		pendingDeletions := []string{}

		for _, line := range hunk.bodyLines {
			switch line.Kind {
			case DELETION:
				// Collect deletions to show before the next non-deletion line
				// Remove the leading "-" from content
				content := line.Content
				if len(content) > 0 && content[0] == '-' {
					content = content[1:]
				}
				pendingDeletions = append(pendingDeletions, content)
			case ADDITION:
				// Flush pending deletions before this addition
				if len(pendingDeletions) > 0 {
					deletedLinesBeforeLine[newLineNum] = append(deletedLinesBeforeLine[newLineNum], pendingDeletions...)
					pendingDeletions = []string{}
				}
				addedLines[newLineNum] = true
				newLineNum++
			case CONTEXT:
				// Flush pending deletions before this context line
				if len(pendingDeletions) > 0 {
					deletedLinesBeforeLine[newLineNum] = append(deletedLinesBeforeLine[newLineNum], pendingDeletions...)
					pendingDeletions = []string{}
				}
				newLineNum++
			}
		}
		// Handle any remaining deletions at the end of the hunk
		if len(pendingDeletions) > 0 {
			// These deletions are at the end, show them after the last line
			deletedLinesBeforeLine[newLineNum] = append(deletedLinesBeforeLine[newLineNum], pendingDeletions...)
		}
	}

	stringBuilder := &strings.Builder{}

	// Calculate line number width
	maxLineNum := len(fileLines)
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))
	if lineNumWidth < 3 {
		lineNumWidth = 3
	}

	// Render each line
	for i, line := range fileLines {
		lineNum := i + 1 // 1-based line number

		// Show any deleted lines before this line
		if deletions, ok := deletedLinesBeforeLine[lineNum]; ok {
			for _, delLine := range deletions {
				prefix := fmt.Sprintf("%s │ ", strings.Repeat(" ", lineNumWidth))
				formattedLine := formatFullFileLine(prefix, "-"+delLine, style.FgRed, false)
				stringBuilder.WriteString(formattedLine + "\n")
			}
		}

		// Show the current line
		lineNumStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)
		prefix := fmt.Sprintf("%s │ ", lineNumStr)

		var lineStyle style.TextStyle
		var content string
		if addedLines[lineNum] {
			lineStyle = style.FgGreen
			content = "+" + line
		} else {
			lineStyle = theme.DefaultTextColor
			content = " " + line
		}

		formattedLine := formatFullFileLine(prefix, content, lineStyle, false)
		stringBuilder.WriteString(formattedLine + "\n")
	}

	// Handle deletions that come after the last line of the file
	if deletions, ok := deletedLinesBeforeLine[len(fileLines)+1]; ok {
		for _, delLine := range deletions {
			prefix := fmt.Sprintf("%s │ ", strings.Repeat(" ", lineNumWidth))
			formattedLine := formatFullFileLine(prefix, "-"+delLine, style.FgRed, false)
			stringBuilder.WriteString(formattedLine + "\n")
		}
	}

	return stringBuilder.String()
}

func formatFullFileLine(prefix string, content string, textStyle style.TextStyle, included bool) string {
	prefixStyle := theme.DefaultTextColor
	firstCharStyle := textStyle
	if included {
		firstCharStyle = firstCharStyle.MergeStyle(style.BgGreen)
	}

	if len(content) == 0 {
		return prefixStyle.Sprint(prefix)
	}

	if len(content) == 1 {
		return prefixStyle.Sprint(prefix) + firstCharStyle.Sprint(content)
	}

	return prefixStyle.Sprint(prefix) + firstCharStyle.Sprint(content[:1]) + textStyle.Sprint(content[1:])
}

func patchLineStyle(patchLine *PatchLine) style.TextStyle {
	switch patchLine.Kind {
	case ADDITION:
		return style.FgGreen
	case DELETION:
		return style.FgRed
	default:
		return theme.DefaultTextColor
	}
}

func (self *patchPresenter) format() string {
	// if we have no changes in our patch (i.e. no additions or deletions) then
	// the patch is effectively empty and we can return an empty string
	if !self.patch.ContainsChanges() {
		return ""
	}

	stringBuilder := &strings.Builder{}
	lineIdx := 0
	appendLine := func(line string) {
		_, _ = stringBuilder.WriteString(line + "\n")

		lineIdx++
	}

	for _, line := range self.patch.header {
		// always passing false for 'included' here because header lines are not part of the patch
		appendLine(self.formatLineAux(line, theme.DefaultTextColor.SetBold(), false))
	}

	for _, hunk := range self.patch.hunks {
		appendLine(
			self.formatLineAux(
				hunk.formatHeaderStart(),
				style.FgCyan,
				false,
			) +
				// we're splitting the line into two parts: the diff header and the context
				// We explicitly pass 'included' as false for both because these are not part
				// of the actual patch
				self.formatLineAux(
					hunk.headerContext,
					theme.DefaultTextColor,
					false,
				),
		)

		for _, line := range hunk.bodyLines {
			style := self.patchLineStyle(line)
			if line.IsChange() {
				appendLine(self.formatLine(line.Content, style, lineIdx))
			} else {
				appendLine(self.formatLineAux(line.Content, style, false))
			}
		}
	}

	return stringBuilder.String()
}

func (self *patchPresenter) patchLineStyle(patchLine *PatchLine) style.TextStyle {
	return patchLineStyle(patchLine)
}

func (self *patchPresenter) formatLine(str string, textStyle style.TextStyle, index int) string {
	included := self.incLineIndices.Includes(index)

	return self.formatLineAux(str, textStyle, included)
}

// 'selected' means you've got it highlighted with your cursor
// 'included' means the line has been included in the patch (only applicable when
// building a patch)
func (self *patchPresenter) formatLineAux(str string, textStyle style.TextStyle, included bool) string {
	if self.plain {
		return str
	}

	firstCharStyle := textStyle
	if included {
		firstCharStyle = firstCharStyle.MergeStyle(style.BgGreen)
	}

	if len(str) < 2 {
		return firstCharStyle.Sprint(str)
	}

	return firstCharStyle.Sprint(str[:1]) + textStyle.Sprint(str[1:])
}
