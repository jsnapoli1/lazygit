package helpers

import (
	"fmt"
	mapsPkg "maps"
	"math"

	"github.com/jesseduffield/lazycore/pkg/boxlayout"
	"github.com/jesseduffield/lazygit/pkg/config"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"golang.org/x/exp/slices"
)

// In this file we use the boxlayout package, along with knowledge about the app's state,
// to arrange the windows (i.e. panels) on the screen.

type WindowArrangementHelper struct {
	c               *HelperCommon
	windowHelper    *WindowHelper
	modeHelper      *ModeHelper
	appStatusHelper *AppStatusHelper
}

func NewWindowArrangementHelper(
	c *HelperCommon,
	windowHelper *WindowHelper,
	modeHelper *ModeHelper,
	appStatusHelper *AppStatusHelper,
) *WindowArrangementHelper {
	return &WindowArrangementHelper{
		c:               c,
		windowHelper:    windowHelper,
		modeHelper:      modeHelper,
		appStatusHelper: appStatusHelper,
	}
}

type WindowArrangementArgs struct {
	// Width of the screen (in characters)
	Width int
	// Height of the screen (in characters)
	Height int
	// User config
	UserConfig *config.UserConfig
	// Name of the currently focused window. (It's actually the current static window, meaning
	// popups are ignored)
	CurrentWindow string
	// Name of the current side window (i.e. the current window in the left
	// section of the UI)
	CurrentSideWindow string
	// Whether the main panel is split (as is the case e.g. when a file has both
	// staged and unstaged changes)
	SplitMainPanel bool
	// The current screen mode (normal, half, full)
	ScreenMode types.ScreenMode
	// The content shown on the bottom left of the screen when showing a loader
	// or toast e.g. 'Rebasing /'
	AppStatus string
	// The content shown on the bottom right of the screen (e.g. the 'donate',
	// 'ask question' links or a message about the current mode e.g. rebase mode)
	InformationStr string
	// Whether to show the extras window which contains the command log context
	ShowExtrasWindow bool
	// Whether we are in a demo (which is used for generating demo gifs for the
	// repo's readme)
	InDemo bool
	// Whether any mode is active (e.g. rebasing, cherry picking, etc)
	IsAnyModeActive bool
	// Whether the search prompt is shown in the bottom left
	InSearchPrompt bool
	// One of '' (not searching), 'Search: ', and 'Filter: '
	SearchPrefix string
}

func (self *WindowArrangementHelper) GetWindowDimensions(informationStr string, appStatus string) map[string]boxlayout.Dimensions {
	width, height := self.c.GocuiGui().Size()
	repoState := self.c.State().GetRepoState()

	var searchPrefix string
	if filterableContext, ok := repoState.GetSearchState().Context.(types.IFilterableContext); ok {
		searchPrefix = filterableContext.FilterPrefix(self.c.Tr)
	} else {
		searchPrefix = self.c.Tr.SearchPrefix
	}

	args := WindowArrangementArgs{
		Width:             width,
		Height:            height,
		UserConfig:        self.c.UserConfig(),
		CurrentWindow:     self.c.Context().CurrentStatic().GetWindowName(),
		CurrentSideWindow: self.c.Context().CurrentSide().GetWindowName(),
		SplitMainPanel:    repoState.GetSplitMainPanel(),
		ScreenMode:        repoState.GetScreenMode(),
		AppStatus:         appStatus,
		InformationStr:    informationStr,
		ShowExtrasWindow:  self.c.State().GetShowExtrasWindow(),
		InDemo:            self.c.InDemo(),
		IsAnyModeActive:   self.modeHelper.IsAnyModeActive(),
		InSearchPrompt:    repoState.InSearchPrompt(),
		SearchPrefix:      searchPrefix,
	}

	return GetWindowDimensions(args)
}

func shouldUsePortraitMode(args WindowArrangementArgs) bool {
	if args.ScreenMode == types.SCREEN_HALF {
		return args.UserConfig.Gui.EnlargedSideViewLocation == "top"
	}

	switch args.UserConfig.Gui.PortraitMode {
	case "never":
		return false
	case "always":
		return true
	default: // "auto" or any garbage values in PortraitMode value
		return args.Width <= 84 && args.Height > 45
	}
}

func GetWindowDimensions(args WindowArrangementArgs) map[string]boxlayout.Dimensions {
	sideSectionWeight, mainSectionWeight := getMidSectionWeights(args)

	sidePanelsDirection := boxlayout.COLUMN
	if shouldUsePortraitMode(args) {
		sidePanelsDirection = boxlayout.ROW
	}

	// Info section only shows for app status or mode indicators now
	// Options/keybinds moved next to commits panel
	// Search prompt is now in the options section
	showInfoSection := args.IsAnyModeActive || args.AppStatus != ""
	infoSectionSize := 0
	if showInfoSection {
		infoSectionSize = 1
	}

	// Determine weights for top section (files/branches + main) vs bottom section (commits + options)
	topSectionWeight, commitsSectionWeight := getVerticalSectionWeights(args)

	root := &boxlayout.Box{
		Direction: boxlayout.ROW,
		Children: []*boxlayout.Box{
			{
				Direction: boxlayout.ROW,
				Weight:    1,
				Children: []*boxlayout.Box{
					{
						Direction: sidePanelsDirection,
						Weight:    topSectionWeight,
						Children: []*boxlayout.Box{
							{
								Direction:           boxlayout.ROW,
								Weight:              sideSectionWeight,
								ConditionalChildren: sidePanelChildren(args),
							},
							{
								Direction: boxlayout.ROW,
								Weight:    mainSectionWeight,
								Children:  mainPanelChildren(args),
							},
						},
					},
					{
						Direction: boxlayout.COLUMN,
						Weight:    commitsSectionWeight,
						Children:  bottomSectionChildren(args),
					},
				},
			},
			{
				Direction: boxlayout.COLUMN,
				Size:      infoSectionSize,
				Children:  infoSectionChildren(args),
			},
		},
	}

	layerOneWindows := boxlayout.ArrangeWindows(root, 0, 0, args.Width, args.Height)
	limitWindows := boxlayout.ArrangeWindows(&boxlayout.Box{Window: "limit"}, 0, 0, args.Width, args.Height)

	return MergeMaps(layerOneWindows, limitWindows)
}

func getVerticalSectionWeights(args WindowArrangementArgs) (int, int) {
	// In full screen mode with commits focused, hide the top section
	if args.ScreenMode == types.SCREEN_FULL && args.CurrentSideWindow == "commits" {
		return 0, 1
	}
	// In full screen mode with files/branches focused, hide the commits section
	if args.ScreenMode == types.SCREEN_FULL && (args.CurrentSideWindow == "files" || args.CurrentSideWindow == "branches") {
		return 1, 0
	}
	// In half screen mode with commits focused, show commits larger
	if args.ScreenMode == types.SCREEN_HALF && args.CurrentSideWindow == "commits" {
		return 1, 2
	}
	// Default: top section gets 2x the height of commits section
	return 2, 1
}

func mainPanelChildren(args WindowArrangementArgs) []*boxlayout.Box {
	mainPanelsDirection := boxlayout.ROW
	if splitMainPanelSideBySide(args) {
		mainPanelsDirection = boxlayout.COLUMN
	}

	result := []*boxlayout.Box{
		{
			Direction: mainPanelsDirection,
			Children:  mainSectionChildren(args),
			Weight:    1,
		},
	}
	if args.ShowExtrasWindow {
		result = append(result, &boxlayout.Box{
			Window: "extras",
			Size:   getExtrasWindowSize(args),
		})
	}
	return result
}

func MergeMaps[K comparable, V any](maps ...map[K]V) map[K]V {
	result := map[K]V{}
	for _, currMap := range maps {
		mapsPkg.Copy(result, currMap)
	}

	return result
}

func mainSectionChildren(args WindowArrangementArgs) []*boxlayout.Box {
	// if we're not in split mode we can just show the one main panel. Likewise if
	// the main panel is focused and we're in full-screen mode
	if !args.SplitMainPanel || (args.ScreenMode == types.SCREEN_FULL && args.CurrentWindow == "main") {
		return []*boxlayout.Box{
			{
				Window: "main",
				Weight: 1,
			},
		}
	}

	if args.CurrentWindow == "secondary" && args.ScreenMode == types.SCREEN_FULL {
		return []*boxlayout.Box{
			{
				Window: "secondary",
				Weight: 1,
			},
		}
	}

	return []*boxlayout.Box{
		{
			Window: "main",
			Weight: 1,
		},
		{
			Window: "secondary",
			Weight: 1,
		},
	}
}

func getMidSectionWeights(args WindowArrangementArgs) (int, int) {
	sidePanelWidthRatio := args.UserConfig.Gui.SidePanelWidth
	// Using 120 so that the default of 0.3333 will remain consistent with previous behavior
	const maxColumnCount = 120
	mainSectionWeight := int(math.Round(maxColumnCount * (1 - sidePanelWidthRatio)))
	sideSectionWeight := int(math.Round(maxColumnCount * sidePanelWidthRatio))

	if splitMainPanelSideBySide(args) {
		mainSectionWeight = sideSectionWeight * 5 // need to shrink side panel to make way for main panels if side-by-side
	}

	if args.CurrentWindow == "main" || args.CurrentWindow == "secondary" {
		if args.ScreenMode == types.SCREEN_HALF || args.ScreenMode == types.SCREEN_FULL {
			sideSectionWeight = 0
		}
	} else {
		if args.ScreenMode == types.SCREEN_HALF {
			if args.UserConfig.Gui.EnlargedSideViewLocation == "top" {
				mainSectionWeight = sideSectionWeight * 2
			} else {
				mainSectionWeight = sideSectionWeight
			}
		} else if args.ScreenMode == types.SCREEN_FULL {
			mainSectionWeight = 0
		}
	}

	return sideSectionWeight, mainSectionWeight
}

func infoSectionChildren(args WindowArrangementArgs) []*boxlayout.Box {
	// This section now only shows for app status or mode indicators
	// Options/keybinds and search moved to the options section next to commits

	statusSpacerPrefix := "statusSpacer"
	spacerBoxIndex := 0
	maxSpacerBoxIndex := 2 // See pkg/gui/types/views.go

	// Returns a box with weight 1 to be used as flexible padding between views
	flexibleSpacerBox := func() *boxlayout.Box {
		spacerBoxIndex++

		if spacerBoxIndex > maxSpacerBoxIndex {
			panic("Too many spacer boxes")
		}

		return &boxlayout.Box{Window: fmt.Sprintf("%s%d", statusSpacerPrefix, spacerBoxIndex), Weight: 1}
	}

	var result []*boxlayout.Box

	if !args.InDemo && args.AppStatus != "" {
		result = append(result, &boxlayout.Box{Window: "appStatus", Size: utils.StringWidth(args.AppStatus)})
	}

	if args.IsAnyModeActive {
		result = append(result,
			&boxlayout.Box{
				Window: "information",
				Size:   utils.StringWidth(utils.Decolorise(args.InformationStr)),
			})
	}

	// Add flexible spacer between status and information if both present
	if len(result) == 2 {
		result = slices.Insert(result, 1, flexibleSpacerBox())
	} else if len(result) == 1 {
		if result[0].Window == "information" {
			// Right-align information
			result = slices.Insert(result, 0, flexibleSpacerBox())
		} else {
			// Status fills the width
			result[0].Size = 0
			result[0].Weight = 1
		}
	}

	return result
}

func splitMainPanelSideBySide(args WindowArrangementArgs) bool {
	if !args.SplitMainPanel {
		return false
	}

	mainPanelSplitMode := args.UserConfig.Gui.MainPanelSplitMode
	switch mainPanelSplitMode {
	case "vertical":
		return false
	case "horizontal":
		return true
	default:
		if args.Width < 200 && args.Height > 30 { // 2 80 character width panels + 40 width for side panel
			return false
		}
		return true
	}
}

func getExtrasWindowSize(args WindowArrangementArgs) int {
	var baseSize int
	// The 'extras' window contains the command log context
	if args.CurrentWindow == "extras" {
		baseSize = 1000 // my way of saying 'fill the available space'
	} else if args.Height < 40 {
		baseSize = 1
	} else {
		baseSize = args.UserConfig.Gui.CommandLogSize
	}

	frameSize := 2
	return baseSize + frameSize
}

func sidePanelChildren(args WindowArrangementArgs) func(width int, height int) []*boxlayout.Box {
	return func(width int, height int) []*boxlayout.Box {
		if args.ScreenMode == types.SCREEN_FULL || args.ScreenMode == types.SCREEN_HALF {
			// In full/half screen mode, only files and branches are in this section
			// If commits is focused, we still show files/branches normally (commits is in its own section)
			if args.CurrentSideWindow == "commits" {
				// When commits is focused, files/branches should still be visible in their section
				return []*boxlayout.Box{
					{Window: "files", Weight: 1},
					{Window: "branches", Weight: 1},
				}
			}

			fullHeightBox := func(window string) *boxlayout.Box {
				if window == args.CurrentSideWindow {
					return &boxlayout.Box{
						Window: window,
						Weight: 1,
					}
				}

				return &boxlayout.Box{
					Window: window,
					Size:   0,
				}
			}

			return []*boxlayout.Box{
				fullHeightBox("files"),
				fullHeightBox("branches"),
			}
		} else if height >= 28 {
			accordionMode := args.UserConfig.Gui.ExpandFocusedSidePanel
			accordionBox := func(defaultBox *boxlayout.Box) *boxlayout.Box {
				if accordionMode && defaultBox.Window == args.CurrentSideWindow {
					return &boxlayout.Box{
						Window: defaultBox.Window,
						Weight: args.UserConfig.Gui.ExpandedSidePanelWeight,
					}
				}

				return defaultBox
			}

			return []*boxlayout.Box{
				accordionBox(&boxlayout.Box{Window: "files", Weight: 1}),
				accordionBox(&boxlayout.Box{Window: "branches", Weight: 1}),
			}
		}

		squashedHeight := 1
		if height >= 21 {
			squashedHeight = 3
		}

		squashedSidePanelBox := func(window string) *boxlayout.Box {
			// For files and branches, check if either is focused
			// (commits is in a separate section so it won't match here)
			if window == args.CurrentSideWindow {
				return &boxlayout.Box{
					Window: window,
					Weight: 1,
				}
			}

			return &boxlayout.Box{
				Window: window,
				Size:   squashedHeight,
			}
		}

		// If commits is focused, both files and branches get squashed
		// We need at least one with Weight to avoid empty weights
		if args.CurrentSideWindow == "commits" {
			return []*boxlayout.Box{
				{Window: "files", Weight: 1},
				{Window: "branches", Weight: 1},
			}
		}

		return []*boxlayout.Box{
			squashedSidePanelBox("files"),
			squashedSidePanelBox("branches"),
		}
	}
}

func commitsPanelChildren(args WindowArrangementArgs) func(width int, height int) []*boxlayout.Box {
	return func(width int, height int) []*boxlayout.Box {
		// In full/half screen mode, show commits at full size when focused
		if (args.ScreenMode == types.SCREEN_FULL || args.ScreenMode == types.SCREEN_HALF) &&
			args.CurrentSideWindow == "commits" {
			return []*boxlayout.Box{
				{Window: "commits", Weight: 1},
			}
		}

		return []*boxlayout.Box{
			{Window: "commits", Weight: 1},
		}
	}
}

func bottomSectionChildren(args WindowArrangementArgs) []*boxlayout.Box {
	// Commits takes 2/3 width, options section takes 1/3
	optionsDirection := boxlayout.ROW
	if args.InSearchPrompt {
		// Search needs horizontal layout (searchPrefix | search)
		optionsDirection = boxlayout.COLUMN
	}
	return []*boxlayout.Box{
		{
			Direction:           boxlayout.ROW,
			Weight:              2,
			ConditionalChildren: commitsPanelChildren(args),
		},
		{
			Direction: optionsDirection,
			Weight:    1,
			Children:  optionsSectionChildren(args),
		},
	}
}

func optionsSectionChildren(args WindowArrangementArgs) []*boxlayout.Box {
	// Show search prompt if active, otherwise show options
	if args.InSearchPrompt {
		return []*boxlayout.Box{
			{
				Window: "searchPrefix",
				Size:   utils.StringWidth(args.SearchPrefix),
			},
			{
				Window: "search",
				Weight: 1,
			},
		}
	}

	// Show options (keybind notes) in this section
	return []*boxlayout.Box{
		{Window: "options", Weight: 1},
	}
}
