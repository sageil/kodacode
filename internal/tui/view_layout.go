package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

type shellLayout struct {
	totalWidth       int
	mainHeight       int
	transcriptHeight int
	inspectorHeight  int
	showSidePanel    bool
	showInspector    bool
	stacked          bool
	sidePanelWidth   int
	centerWidth      int
	rightWidth       int
	terminalHeight   int
	terminalVisible  bool
	contentHeight    int // inner content height (mainHeight-2 for the wide shell box, else mainHeight)
}

type shellRects struct {
	header        inputMouseRect
	body          inputMouseRect
	transcript    inputMouseRect
	inspector     inputMouseRect
	sessions      inputMouseRect
	composerFocus inputMouseRect
}

func resolveShellRects(m Model, state events.SessionState, layout shellLayout) shellRects {
	if m.width <= 0 || m.height <= 0 {
		return shellRects{}
	}

	if isWideShell(m) {
		layout = normalizeWideShellLayout(m, state, layout)
		headerHeight := lipgloss.Height(renderSplitWideHeader(m, state, layout.totalWidth))
		bodyHeight := splitWidePanelHeight(layout)
		transcriptWidth := layout.centerWidth
		if !layout.showInspector {
			transcriptWidth = layout.totalWidth
		}

		rects := shellRects{
			header: inputMouseRect{
				x:      0,
				y:      0,
				width:  layout.totalWidth,
				height: headerHeight,
			},
			body: inputMouseRect{
				x:      0,
				y:      headerHeight,
				width:  layout.totalWidth,
				height: bodyHeight,
			},
			transcript: inputMouseRect{
				x:      0,
				y:      headerHeight,
				width:  transcriptWidth,
				height: bodyHeight,
			},
			composerFocus: inputMouseRect{
				x:      0,
				y:      headerHeight + bodyHeight,
				width:  max(layout.totalWidth, 1),
				height: max(m.height-headerHeight-bodyHeight, 0),
			},
		}
		if layout.showInspector {
			rects.inspector = inputMouseRect{
				x:      layout.centerWidth + 1,
				y:      headerHeight,
				width:  layout.rightWidth,
				height: bodyHeight,
			}
		}
		return rects
	}

	headerHeight := lipgloss.Height(renderHeaderBar(m, state, layout.totalWidth))
	bodyY := headerHeight
	bodyHeight := layout.mainHeight
	rects := shellRects{
		header: inputMouseRect{
			x:      0,
			y:      0,
			width:  layout.totalWidth,
			height: headerHeight,
		},
		body: inputMouseRect{
			x:      0,
			y:      bodyY,
			width:  layout.totalWidth,
			height: bodyHeight,
		},
		composerFocus: inputMouseRect{
			x:      0,
			y:      max(bodyY+bodyHeight, 0),
			width:  max(layout.totalWidth, 1),
			height: max(m.height-bodyY-bodyHeight, 0),
		},
	}

	x := 0
	if layout.showSidePanel && layout.sidePanelWidth > 0 {
		rects.sessions = inputMouseRect{
			x:      0,
			y:      bodyY,
			width:  layout.sidePanelWidth,
			height: bodyHeight,
		}
		x = layout.sidePanelWidth + 1
	}

	if layout.stacked {
		rects.transcript = inputMouseRect{
			x:      x,
			y:      bodyY,
			width:  layout.centerWidth,
			height: layout.transcriptHeight,
		}
		if layout.showInspector && layout.inspectorHeight > 0 {
			rects.inspector = inputMouseRect{
				x:      x,
				y:      bodyY + layout.transcriptHeight,
				width:  layout.rightWidth,
				height: layout.inspectorHeight,
			}
		}
		return rects
	}

	rects.transcript = inputMouseRect{
		x:      x,
		y:      bodyY,
		width:  layout.centerWidth,
		height: bodyHeight,
	}
	if layout.showInspector {
		rects.inspector = inputMouseRect{
			x:      x + layout.centerWidth + 1,
			y:      bodyY,
			width:  layout.rightWidth,
			height: bodyHeight,
		}
	}
	return rects
}

func (r shellRects) mouseTargetAt(mouseX, mouseY int) mouseWheelTarget {
	switch {
	case r.sessions.contains(mouseX, mouseY):
		return mouseWheelTargetSessions
	case r.transcript.contains(mouseX, mouseY):
		return mouseWheelTargetTranscript
	case r.inspector.contains(mouseX, mouseY):
		return mouseWheelTargetInspector
	default:
		return mouseWheelTargetNone
	}
}

func resolveShellLayout(m Model, state events.SessionState) shellLayout {
	totalWidth := max(terminalWidth(m), 1)
	forcedInspector := m.hasPendingApproval()
	showInspector := forcedInspector || m.chrome.inspectorOpen

	if totalWidth >= 126 {
		showInspector = forcedInspector || m.chrome.wideSidebarOpen
		sidePanelWidth := 0
		showSidePanel := false
		rightWidth := totalWidth * 24 / 100
		dividerCount := 0
		if showInspector {
			rightWidth = max(rightWidth, 28)
			dividerCount = 1
		}
		sidePanelWidth, rightWidth, centerWidth := rebalanceShellWidths(totalWidth, dividerCount, 0, sidePanelWidth, rightWidth, showSidePanel, showInspector, 56)
		contentH := max(m.height-splitWideHeaderHeight()-splitWideFooterHeight(m, state, totalWidth), 1)
		return shellLayout{
			totalWidth:       totalWidth,
			mainHeight:       contentH,
			transcriptHeight: contentH,
			inspectorHeight:  contentH,
			showSidePanel:    showSidePanel,
			showInspector:    showInspector,
			sidePanelWidth:   sidePanelWidth,
			centerWidth:      centerWidth,
			rightWidth:       rightWidth,
			contentHeight:    contentH,
		}
	}

	mainHeight := mainShellHeight(m, state, totalWidth)
	transcriptHeight := mainHeight
	inspectorHeight := mainHeight

	switch {
	case totalWidth >= 92:
		dividerCount := 0
		rightWidth := 0
		if showInspector {
			rightWidth = max(totalWidth*32/100, 26)
			dividerCount++
		}
		_, rightWidth, centerWidth := rebalanceShellWidths(totalWidth, dividerCount, 0, 0, rightWidth, false, showInspector, 36)
		return shellLayout{
			totalWidth:       totalWidth,
			mainHeight:       mainHeight,
			transcriptHeight: transcriptHeight,
			inspectorHeight:  inspectorHeight,
			showInspector:    showInspector,
			centerWidth:      centerWidth,
			rightWidth:       rightWidth,
			contentHeight:    mainHeight,
		}
	default:
		if showInspector {
			transcriptHeight, inspectorHeight = splitStackedHeights(mainHeight)
		}
		return shellLayout{
			totalWidth:       totalWidth,
			mainHeight:       mainHeight,
			transcriptHeight: transcriptHeight,
			inspectorHeight:  inspectorHeight,
			showInspector:    showInspector,
			stacked:          showInspector,
			centerWidth:      totalWidth,
			rightWidth:       totalWidth,
			contentHeight:    mainHeight,
		}
	}
}

func rebalanceShellWidths(totalWidth, dividerCount, fixedWidth, panelWidth, rightWidth int, showPanel, showInspector bool, centerMin int) (int, int, int) {
	if !showPanel {
		panelWidth = 0
	}
	if !showInspector {
		rightWidth = 0
	}

	centerWidth := totalWidth - dividerCount - fixedWidth - panelWidth - rightWidth
	if centerWidth >= centerMin {
		return panelWidth, rightWidth, centerWidth
	}

	shortage := centerMin - centerWidth
	if showInspector {
		shrink := min(shortage, max(rightWidth-totalWidth/5, 0))
		rightWidth -= shrink
		shortage -= shrink
	}
	if shortage > 0 && showPanel {
		shrink := min(shortage, max(panelWidth-16, 0))
		panelWidth -= shrink
	}

	centerWidth = totalWidth - dividerCount - fixedWidth - panelWidth - rightWidth
	return max(panelWidth, 0), max(rightWidth, 0), max(centerWidth, 1)
}

func mainShellHeight(m Model, state events.SessionState, width int) int {
	if m.height <= 0 {
		return 1
	}
	headerHeight := lipgloss.Height(renderHeaderBar(m, state, width))
	footerHeight := lipgloss.Height(renderFooterBar(m, state, width))
	height := m.height - headerHeight - footerHeight
	return max(height, 1)
}

func splitStackedHeights(totalHeight int) (int, int) {
	if totalHeight <= 1 {
		return 1, 0
	}
	inspectorHeight := max(totalHeight/3, 8)
	transcriptHeight := totalHeight - inspectorHeight
	if transcriptHeight < 10 {
		transcriptHeight = max(totalHeight-8, 1)
		inspectorHeight = totalHeight - transcriptHeight
	}
	if inspectorHeight < 1 {
		inspectorHeight = 1
		transcriptHeight = max(totalHeight-inspectorHeight, 1)
	}
	return transcriptHeight, inspectorHeight
}

func renderMainShell(m Model, state events.SessionState, layout shellLayout) string {
	bodyTone := toneBG
	content := renderMainShellContent(m, state, layout, bodyTone)
	return renderToneBlock(m.theme, bodyTone, max(layout.totalWidth, 1), max(layout.mainHeight, 1), content)
}

func renderMainShellContent(m Model, state events.SessionState, layout shellLayout, bodyTone string) string {
	panelTone := tonePanel
	transcriptTone := bodyTone
	inspectorTone := tonePanelAlt
	centerContentWidth := max(layout.centerWidth, 1)

	if layout.stacked {
		transcript := renderToneBlock(m.theme, transcriptTone, layout.centerWidth, max(layout.transcriptHeight, 1), renderTranscriptPane(m, state, centerContentWidth))
		sections := []string{transcript}
		if layout.showInspector && layout.inspectorHeight > 0 {
			rightContentWidth := max(layout.rightWidth, 1)
			sections = append(sections, renderToneBlock(m.theme, inspectorTone, layout.rightWidth, layout.inspectorHeight, renderInspectorPane(m, state, rightContentWidth)))
		}
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	primaryColor := colorFor(m.theme, "primary", "#7cc7ff")
	contentH := max(layout.contentHeight, 1)
	columns := make([]string, 0, 9)

	// Side panel
	if layout.showSidePanel && layout.sidePanelWidth > 0 {
		columns = append(columns, renderToneBlock(m.theme, panelTone, layout.sidePanelWidth, contentH, renderSidePanel(m, state, max(layout.sidePanelWidth, 1))))
		panelDivColor := ""
		if m.chrome.focus == focusTranscript {
			panelDivColor = primaryColor
		}
		columns = append(columns, wideColumnDivider(m, contentH, panelDivColor))
	}

	// Transcript — optionally split with terminal pane at bottom
	var transcriptContent string
	if layout.terminalVisible && layout.terminalHeight > 0 {
		upperHeight := max(layout.transcriptHeight-layout.terminalHeight, 1)
		upperBlock := renderToneBlock(m.theme, transcriptTone, layout.centerWidth, upperHeight,
			renderTranscriptPane(m, state, centerContentWidth))
		termBlock := renderToneBlock(m.theme, inspectorTone, layout.centerWidth, layout.terminalHeight,
			renderWideTerminalPane(m, state, centerContentWidth))
		transcriptContent = lipgloss.JoinVertical(lipgloss.Left, upperBlock, termBlock)
	} else {
		transcriptContent = renderToneBlock(m.theme, transcriptTone, layout.centerWidth, max(layout.transcriptHeight, 1),
			renderTranscriptPane(m, state, centerContentWidth))
	}
	columns = append(columns, transcriptContent)

	// Right panel — inspector or wide tools/tasks panel
	if layout.showInspector {
		var divider string
		rightDivColor := ""
		if m.chrome.focus == focusInspector || m.chrome.focus == focusTranscript {
			rightDivColor = primaryColor
		}
		divider = shellColumnDivider(m, contentH, rightDivColor)
		columns = append(columns, divider)
		rightContentWidth := max(layout.rightWidth, 1)
		var rightPanel string
		if m.hasPendingApproval() || strings.TrimSpace(m.selection.handoffID) != "" {
			rightPanel = renderInspectorPane(m, state, rightContentWidth)
		} else if isWideShell(m) {
			rightPanel = renderWideRightPanel(m, state, rightContentWidth)
		} else {
			rightPanel = renderInspectorPane(m, state, rightContentWidth)
		}
		columns = append(columns, renderToneBlock(m.theme, inspectorTone, layout.rightWidth, contentH, rightPanel))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, columns...)
}
