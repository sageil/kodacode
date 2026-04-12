package tui

import (
	"fmt"
	"log"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// diffReview holds the per-hunk acceptance state for an edit/write tool call.
type diffReview struct {
	hunks    []DiffHunk
	allOps   []diffOp
	filePath string
	oldLines []string
	newStr   string
	isEdit   bool
	dirty    bool
	applied  bool   // true after successfully applying changes
	status   string // status message shown in header after apply
}

// diffReviewMeta carries line-offset information from renderToolPanelContent
// back to the render() assembly so it can build hunkRegions.
type diffReviewMeta struct {
	msgIndex     int
	hunkOffsets  []int
	actionOffset int
}

// applyDiffReview writes the selectively-patched file based on hunk acceptance state.
// Uses value receiver because it only mutates the diffReviews map (reference type).
func (m Messages) applyDiffReview(msgIndex int) {
	dr, ok := m.diffReviews[msgIndex]
	if !ok || !dr.dirty || dr.applied {
		return
	}

	accepted := 0
	for _, h := range dr.hunks {
		if h.Accepted {
			accepted++
		}
	}

	content := reconstructContent(dr.allOps, dr.hunks)

	if dr.isEdit && dr.newStr != "" {
		data, err := os.ReadFile(dr.filePath)
		if err != nil {
			log.Printf("applyDiffReview: read %s: %v", dr.filePath, err)
			dr.status = "error: could not read file"
			return
		}
		current := string(data)
		if !strings.Contains(current, dr.newStr) {
			log.Printf("applyDiffReview: newString not found in %s (file may have changed)", dr.filePath)
			dr.status = "error: file was modified externally"
			return
		}
		patched := strings.Replace(current, dr.newStr, content, 1)
		if err := os.WriteFile(dr.filePath, []byte(patched), 0o644); err != nil {
			log.Printf("applyDiffReview: write %s: %v", dr.filePath, err)
			dr.status = "error: could not write file"
			return
		}
	} else {
		if err := os.WriteFile(dr.filePath, []byte(content+"\n"), 0o644); err != nil {
			log.Printf("applyDiffReview: write %s: %v", dr.filePath, err)
			dr.status = "error: could not write file"
			return
		}
	}

	dr.dirty = false
	dr.applied = true
	dr.status = fmt.Sprintf("applied %d/%d hunks", accepted, len(dr.hunks))
	log.Printf("applyDiffReview: wrote %s (%s)", dr.filePath, dr.status)
}

// renderDiffWithHunks renders a diff with per-hunk accept/reject controls.
// It initializes the diffReview state lazily and renders hunk headers.
// Returns diffReviewMeta for building click regions.
func (m *Messages) renderDiffWithHunks(body *strings.Builder, msgIndex int,
	allOps []diffOp, oldLines []string, filePath, newStr string, isEdit bool,
	_ lipgloss.Style, textWidth int,
) *diffReviewMeta {
	dr, ok := m.diffReviews[msgIndex]
	if !ok {
		hunks := splitIntoHunks(allOps, 3)
		if len(hunks) == 0 {
			// No changes — just render the diff normally.
			for _, line := range renderDiffOps(trimDiffContext(allOps, 3), textWidth) {
				fmt.Fprintf(body, "  %s\n", line)
			}
			return nil
		}
		dr = &diffReview{
			hunks:    hunks,
			allOps:   allOps,
			filePath: filePath,
			oldLines: oldLines,
			newStr:   newStr,
			isEdit:   isEdit,
		}
		m.diffReviews[msgIndex] = dr
	}

	meta := &diffReviewMeta{
		msgIndex:     msgIndex,
		actionOffset: -1,
	}

	// After applying, show status above the diff.
	if dr.status != "" {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#66cc66"))
		if strings.HasPrefix(dr.status, "error:") {
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cc6666"))
		}
		fmt.Fprintf(body, "  %s\n", statusStyle.Render(dr.status))
	}

	// Styles for hunk headers.
	acceptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#66cc66")).Bold(true)
	rejectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cc6666")).Bold(true)
	hdrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6699cc"))
	actionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Bold(true)
	dimOps := lipgloss.NewStyle().Faint(true)

	lineOffset := 0 // tracks lines written to body (for meta offsets)

	for hi, hunk := range dr.hunks {
		// Hunk header line.
		var indicator string
		if hunk.Accepted {
			indicator = acceptStyle.Render("✓")
		} else {
			indicator = rejectStyle.Render("✗")
		}
		label := fmt.Sprintf("Hunk %d/%d", hi+1, len(dr.hunks))
		left := "── " + label + " "
		right := " ──"
		indicatorW := lipgloss.Width(indicator)
		fillW := max(textWidth-len([]rune(left))-indicatorW-len([]rune(right))-1, 0)
		sep := hdrStyle.Render(left+strings.Repeat("─", fillW)) + " " + indicator + hdrStyle.Render(right)
		if sw := lipgloss.Width(sep); sw < textWidth {
			sep += strings.Repeat(" ", textWidth-sw)
		}
		meta.hunkOffsets = append(meta.hunkOffsets, lineOffset)
		fmt.Fprintf(body, "  %s\n", sep)
		lineOffset++

		// Render hunk diff ops — dim if rejected.
		trimmed := trimDiffContext(hunk.Ops, 3)
		rendered := renderDiffOps(trimmed, textWidth)
		for _, line := range rendered {
			if !hunk.Accepted {
				line = dimOps.Render(line)
			}
			fmt.Fprintf(body, "  %s\n", line)
			lineOffset++
		}
	}

	// Action bar — only shown when at least one hunk was toggled.
	if dr.dirty {
		actionLabel := actionStyle.Render("[Apply Changes]")
		aPrefix := "── "
		aFillW := max(textWidth-lipgloss.Width(aPrefix)-lipgloss.Width(actionLabel)-1, 0)
		actionSep := hdrStyle.Render(aPrefix) + actionLabel + " " +
			hdrStyle.Render(strings.Repeat("─", aFillW))
		if sw := lipgloss.Width(actionSep); sw < textWidth {
			actionSep += strings.Repeat(" ", textWidth-sw)
		}
		meta.actionOffset = lineOffset
		fmt.Fprintf(body, "  %s\n", actionSep)
	}

	return meta
}
