package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (d *commandPaletteDialog) renderPaletteModelHeaderLine(prefix string) string {
	return dialogHintStyle(d.theme).Render(prefix + paletteModelHeaderPlainLine())
}

func (d *commandPaletteDialog) renderPaletteModelLine(item modelItem, selected bool, prefix, description string, disabled bool) string {
	normal := dialogItemStyle(d.theme)
	selectedStyle := dialogSelectedItemStyle(d.theme)
	dim := dialogHintStyle(d.theme)
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(d.theme, "secondary", "#7dcfff")))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color(colorFor(d.theme, "success", "#90e5b4")))
	providerName, modelName := paletteModelDisplayNames(item)

	if disabled {
		row := dim.Render(prefix + providerName + "  " + modelName)
		if strings.TrimSpace(description) != "" {
			row += "  " + dim.Render(strings.TrimSpace(description))
		}
		return row
	}

	ctx := formatPaletteContextSize(item.Capacity.InputTokens)
	window := paletteModelWindowText(item)
	price := formatPalettePrice(item.CostInput, item.CostOutput)

	reasoning := dim.Render("·")
	if item.Reasoning {
		reasoning = green.Render("R")
	}
	toolCalls := dim.Render("·")
	if item.ToolCalls {
		toolCalls = green.Render("T")
	}
	vision := dim.Render("·")
	if item.Vision {
		vision = green.Render("V")
	}
	caps := reasoning + " " + toolCalls + " " + vision

	providerCol := accent.Render(fmt.Sprintf("%-*s", paletteModelColProvider, providerName))
	modelColText := fmt.Sprintf("%-*s", paletteModelColModel, modelName)
	ctxCol := dim.Render(fmt.Sprintf("%*s", paletteModelColCtx, ctx))
	windowCol := dim.Render(fmt.Sprintf("%*s", paletteModelColWindow, window))
	priceCol := dim.Render(fmt.Sprintf("%*s", paletteModelColPrice, price))
	if selected {
		modelCol := selectedStyle.Render(modelColText)
		row := selectedStyle.Render(prefix) + providerCol + " " + modelCol + " " + ctxCol + " " + windowCol + " " + priceCol + " " + caps
		if strings.TrimSpace(description) != "" {
			row += "  " + dim.Render(strings.TrimSpace(description))
		}
		return row
	}
	modelCol := normal.Render(modelColText)
	row := normal.Render(prefix) + providerCol + " " + modelCol + " " + ctxCol + " " + windowCol + " " + priceCol + " " + caps
	if strings.TrimSpace(description) != "" {
		row += "  " + dim.Render(strings.TrimSpace(description))
	}
	return row
}
