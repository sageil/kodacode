package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

func renderReviewTranscriptSection(m Model, turn *events.TurnState, width int) string {
	if turn == nil || turn.Review == nil {
		return ""
	}
	cardBG := toneValue(m.theme, tonePanelAlt)
	accent := colorFor(m.theme, "secondary", "#7dcfff")
	signature := reviewTranscriptPlainText(turn)
	return cachedTranscriptRender("review_card", m, width, func() string {
		blockWidth := max(width, 1)
		hPadding := 2
		contentWidth := max(blockWidth-hPadding*2, 1)
		contentLines := renderReviewContentLines(m, turn, contentWidth, "")
		for i, line := range contentLines {
			contentLines[i] = fillBackground(contentWidth, cardBG, line)
		}
		content := strings.Join(contentLines, "\n")
		return lipgloss.NewStyle().
			Width(blockWidth).
			Padding(1, hPadding).
			Background(lipgloss.Color(cardBG)).
			Render(persistBackgroundANSI(content, cardBG))
	}, strings.TrimSpace(cardBG), accent, signature)
}

func renderReviewContentLines(m Model, turn *events.TurnState, width int, bg string) []string {
	if turn == nil || turn.Review == nil {
		return nil
	}
	review := turn.Review
	accent := colorFor(m.theme, "secondary", "#7dcfff")
	lines := make([]string, 0, len(review.Findings)*3+6)
	lines = append(lines, renderTranscriptRuleTitleLine(reviewTranscriptTitle(review), width, accent, bg))
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "secondary", "#7dcfff"))).
		Bold(true)
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	if strings.TrimSpace(bg) != "" {
		bgColor := lipgloss.Color(bg)
		labelStyle = labelStyle.Background(bgColor)
		valueStyle = valueStyle.Background(bgColor)
	}

	if modelName := reviewTurnModel(turn); modelName != "" {
		lines = append(lines, "")
		lines = appendWrappedAssistantLine(lines, labelStyle.Render("Model: ")+valueStyle.Render(modelName), width, 0)
	}

	if len(lines) > 0 {
		lines = append(lines, "")
	}
	findings := assistantFindingsFromStructuredReview(review)
	if len(findings) > 0 {
		lines = append(lines, renderAssistantFindingsBlock(m, findings, width, bg)...)
	} else {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
			Bold(true)
		if strings.TrimSpace(bg) != "" {
			titleStyle = titleStyle.Background(lipgloss.Color(bg))
		}
		lines = appendWrappedAssistantLine(lines, titleStyle.Render("No review findings."), width, 0)
	}

	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = appendWrappedAssistantLine(lines, labelStyle.Render("Overall correctness: ")+valueStyle.Render(review.OverallCorrectness), width, 0)
	lines = appendWrappedAssistantLine(lines, labelStyle.Render("Overall summary: ")+valueStyle.Render(review.OverallSummary), width, 0)
	return lines
}

func reviewTranscriptSelectionLines(m Model, turn *events.TurnState, width int) []transcriptSelectionLine {
	blockWidth := max(width, 1)
	hPadding := 2
	contentWidth := max(blockWidth-hPadding*2, 1)
	contentLines := renderReviewContentLines(m, turn, contentWidth, "")
	lines := make([]transcriptSelectionLine, 0, len(contentLines)+2)
	lines = append(lines, transcriptSelectionLine{})
	for _, line := range contentLines {
		lines = append(lines, newTranscriptSelectionLine(ansi.Strip(line), hPadding))
	}
	lines = append(lines, transcriptSelectionLine{})
	return lines
}

func reviewTranscriptPlainText(turn *events.TurnState) string {
	if turn == nil || turn.Review == nil {
		return ""
	}
	review := turn.Review
	lines := make([]string, 0, len(review.Findings)*2+5)
	lines = append(lines, reviewTranscriptTitle(review), "")
	if modelName := reviewTurnModel(turn); modelName != "" {
		lines = append(lines, "Model: "+modelName, "")
	}
	if len(review.Findings) == 0 {
		lines = append(lines, "No review findings.")
	} else {
		for idx, finding := range review.Findings {
			lines = append(lines, strconv.Itoa(idx+1)+". ["+finding.Severity+"] "+finding.Path+":"+strconv.Itoa(finding.Line)+" "+finding.Title)
			lines = append(lines, finding.Explanation)
		}
	}
	lines = append(lines, "Overall correctness: "+review.OverallCorrectness)
	lines = append(lines, "Overall summary: "+review.OverallSummary)
	return strings.Join(lines, "\n")
}

func reviewTurnModel(turn *events.TurnState) string {
	if turn == nil || turn.Config == nil {
		return ""
	}
	return strings.TrimSpace(turn.Config.Model)
}

func reviewTranscriptTitle(review *events.ReviewState) string {
	if review == nil {
		return "Review"
	}
	if title := strings.TrimSpace(review.Title); title != "" {
		return title
	}
	return "Review"
}

func assistantFindingsFromStructuredReview(review *events.ReviewState) []assistantFinding {
	if review == nil || len(review.Findings) == 0 {
		return nil
	}
	findings := make([]assistantFinding, 0, len(review.Findings))
	for _, finding := range review.Findings {
		findings = append(findings, assistantFinding{
			title:   "[" + finding.Severity + "] " + finding.Path + ":" + strconv.Itoa(finding.Line) + " " + finding.Title,
			details: []string{finding.Explanation},
		})
	}
	return findings
}
