package tui

import (
	"fmt"
	"strconv"
	"strings"
)

type stepToolTraceTUI struct {
	Name    string `json:"name"`
	Elapsed int64  `json:"elapsed_ms"`
	Error   string `json:"error,omitempty"`
}

type stepTraceUsageTUI struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type segmentBytesTUI struct {
	StablePrompt int `json:"stable_prompt"`
	SemiStable   int `json:"semi_stable"`
	Volatile     int `json:"volatile"`
	Messages     int `json:"messages"`
	ToolSchemas  int `json:"tool_schemas"`
	Total        int `json:"total"`
}

type stepTraceTUI struct {
	Step         int                `json:"step"`
	ModelID      string             `json:"model_id"`
	Usage        *stepTraceUsageTUI `json:"usage,omitempty"`
	CostMicroUSD int64              `json:"cost_micro_usd"`
	Tools        []stepToolTraceTUI `json:"tools,omitempty"`
	RetryCount   int                `json:"retry_count"`
	FallbackUsed bool               `json:"fallback_used,omitempty"`
	LoopVerdict  int                `json:"loop_verdict,omitempty"`
	WallClock    int64              `json:"wall_clock_ms"`
	Segments     *segmentBytesTUI   `json:"segments,omitempty"`
}

func (a App) renderTurnSummaryTable() string {
	var sb strings.Builder
	sb.WriteString("<table><tr>")
	sb.WriteString("<th>Turn</th><th>In</th><th>Out</th><th>Cost</th><th>Steps</th><th>Tools</th><th>Wall</th>")
	sb.WriteString("</tr>")

	for turnIdx, steps := range a.stepTraces {
		var inTok, outTok, toolCount int
		var costMicro, wallMs int64
		for _, s := range steps {
			if s.Usage != nil {
				inTok += s.Usage.InputTokens + s.Usage.CacheReadTokens + s.Usage.CacheWriteTokens
				outTok += s.Usage.OutputTokens
			}
			costMicro += s.CostMicroUSD
			wallMs += s.WallClock
			toolCount += len(s.Tools)
		}

		sb.WriteString("<tr>")
		fmt.Fprintf(&sb, "<td>%d</td>", turnIdx+1)
		fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(inTok))
		fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(outTok))
		fmt.Fprintf(&sb, "<td>$%.4f</td>", float64(costMicro)/1e6)
		fmt.Fprintf(&sb, "<td>%d</td>", len(steps))
		fmt.Fprintf(&sb, "<td>%d</td>", toolCount)
		fmt.Fprintf(&sb, "<td>%s</td>", formatDuration(wallMs))
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")
	return renderHTMLTable(sb.String()) + "\n`/trace N` for step detail"
}

func (a App) renderTraceDetail(turnArg string) string {
	if !a.traceEnabled {
		return "Trace capture is disabled. Enable `session.trace: true` in config to collect per-turn traces."
	}
	idx, err := strconv.Atoi(turnArg)
	if err != nil || idx < 1 || idx > len(a.stepTraces) {
		return fmt.Sprintf("Invalid turn number %q. Valid range: 1-%d", turnArg, len(a.stepTraces))
	}
	steps := a.stepTraces[idx-1]
	if len(steps) == 0 {
		return fmt.Sprintf("Turn %d has no steps.", idx)
	}

	tokenTable := renderTokenDetail(steps)
	segTable := renderSegmentDetail(steps)
	toolTable := renderToolDetail(steps)

	return fmt.Sprintf("**Turn %d Detail** (%d steps)\n%s%s%s",
		idx, len(steps), tokenTable, segTable, toolTable)
}

func renderTokenDetail(steps []stepTraceTUI) string {
	var sb strings.Builder
	sb.WriteString("<table><tr>")
	sb.WriteString("<th>Step</th><th>In</th><th>Out</th><th>Cache R</th><th>Cache W</th>")
	sb.WriteString("<th>Cost</th><th>Wall</th><th>Notes</th>")
	sb.WriteString("</tr>")
	for _, s := range steps {
		sb.WriteString("<tr>")
		fmt.Fprintf(&sb, "<td>%d</td>", s.Step)
		var inTok, outTok, cacheR, cacheW int
		if s.Usage != nil {
			inTok = s.Usage.InputTokens
			outTok = s.Usage.OutputTokens
			cacheR = s.Usage.CacheReadTokens
			cacheW = s.Usage.CacheWriteTokens
		}
		fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(inTok))
		fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(outTok))
		fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(cacheR))
		fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(cacheW))
		fmt.Fprintf(&sb, "<td>$%.4f</td>", float64(s.CostMicroUSD)/1e6)
		fmt.Fprintf(&sb, "<td>%s</td>", formatDuration(s.WallClock))
		var notes []string
		if s.FallbackUsed {
			notes = append(notes, "fallback")
		}
		if s.RetryCount > 0 {
			notes = append(notes, fmt.Sprintf("retries:%d", s.RetryCount))
		}
		if s.LoopVerdict > 0 {
			notes = append(notes, fmt.Sprintf("loop:%d", s.LoopVerdict))
		}
		fmt.Fprintf(&sb, "<td>%s</td>", strings.Join(notes, " "))
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")
	return renderHTMLTable(sb.String())
}

func renderSegmentDetail(steps []stepTraceTUI) string {
	hasSegments := false
	for _, s := range steps {
		if s.Segments != nil {
			hasSegments = true
			break
		}
	}
	if !hasSegments {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<table><tr>")
	sb.WriteString("<th>Step</th><th>Stable</th><th>SemiStab</th><th>Volatile</th>")
	sb.WriteString("<th>Messages</th><th>Tools</th><th>Tokens</th>")
	sb.WriteString("</tr>")
	for _, s := range steps {
		sb.WriteString("<tr>")
		fmt.Fprintf(&sb, "<td>%d</td>", s.Step)
		if s.Segments == nil {
			for range 6 {
				sb.WriteString("<td>-</td>")
			}
		} else {
			seg := s.Segments
			total := seg.Total
			fmtPct := func(n int) string {
				if total == 0 {
					return "-"
				}
				return fmt.Sprintf("%d%%", n*100/total)
			}
			fmt.Fprintf(&sb, "<td>%s</td>", fmtPct(seg.StablePrompt))
			fmt.Fprintf(&sb, "<td>%s</td>", fmtPct(seg.SemiStable))
			fmt.Fprintf(&sb, "<td>%s</td>", fmtPct(seg.Volatile))
			fmt.Fprintf(&sb, "<td>%s</td>", fmtPct(seg.Messages))
			fmt.Fprintf(&sb, "<td>%s</td>", fmtPct(seg.ToolSchemas))
			// Show actual token total from API usage
			var actualTokens int
			if s.Usage != nil {
				actualTokens = s.Usage.InputTokens + s.Usage.CacheReadTokens + s.Usage.CacheWriteTokens
			}
			fmt.Fprintf(&sb, "<td>%s</td>", formatTraceTokens(actualTokens))
		}
		sb.WriteString("</tr>")
	}
	sb.WriteString("</table>")
	return "\n**Prompt Segments** (% of input by bytes)\n" + renderHTMLTable(sb.String())
}

func renderToolDetail(steps []stepTraceTUI) string {
	hasTools := false
	for _, s := range steps {
		if len(s.Tools) > 0 {
			hasTools = true
			break
		}
	}
	if !hasTools {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<table><tr><th>Step</th><th>Tool</th><th>Elapsed</th><th>Status</th></tr>")
	for _, s := range steps {
		for _, t := range s.Tools {
			sb.WriteString("<tr>")
			fmt.Fprintf(&sb, "<td>%d</td>", s.Step)
			fmt.Fprintf(&sb, "<td>%s</td>", t.Name)
			fmt.Fprintf(&sb, "<td>%s</td>", formatDuration(t.Elapsed))
			status := "ok"
			if t.Error != "" {
				status = "error"
			}
			fmt.Fprintf(&sb, "<td>%s</td>", status)
			sb.WriteString("</tr>")
		}
	}
	sb.WriteString("</table>")
	return "\n**Tool Timing**\n" + renderHTMLTable(sb.String())
}

func formatTraceTokens(n int) string {
	if n == 0 {
		return "-"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func countSteps(traces [][]stepTraceTUI) int {
	n := 0
	for _, steps := range traces {
		n += len(steps)
	}
	return n
}
