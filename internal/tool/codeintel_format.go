package tool

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func formatDefinitionLocations(target string, locations []CodeIntelLocation) string {
	if len(locations) == 0 {
		return "No definition found."
	}
	lines := make([]string, 0, len(locations)+1)
	lines = append(lines, "Definition for "+target)
	for _, location := range locations {
		lines = append(lines, fmt.Sprintf("- %s:%d:%d", location.Path, location.Line, location.Character))
	}
	return strings.Join(lines, "\n")
}

func formatCodeIntelNotice(toolName string, notice *CodeIntelNotice) string {
	if notice == nil {
		return ""
	}
	kind := strings.TrimSpace(string(notice.Kind))
	if kind == "" {
		kind = string(CodeIntelNoticeKindUnavailable)
	}
	message := strings.TrimSpace(notice.Message)
	if message == "" {
		return fmt.Sprintf("%s %s.", toolName, kind)
	}
	return fmt.Sprintf("%s %s: %s", toolName, kind, message)
}

func FormatDiagnostics(files []CodeIntelFileDiagnostics) string {
	if len(files) == 0 {
		return "No diagnostics found."
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	sections := make([]string, 0, len(files))
	for _, file := range files {
		section := formatFileDiagnostics(file)
		if strings.TrimSpace(section) != "" {
			sections = append(sections, section)
		}
	}
	if len(sections) == 0 {
		return "No diagnostics found."
	}
	return strings.Join(sections, "\n\n")
}

func formatDiagnostics(files []CodeIntelFileDiagnostics) string {
	return FormatDiagnostics(files)
}

func formatFileDiagnostics(file CodeIntelFileDiagnostics) string {
	filtered := filterDiagnosticsForFile(file.Path, file.Diagnostics)
	if len(filtered) == 0 && strings.TrimSpace(file.Error) == "" {
		return ""
	}
	lines := make([]string, 0, len(filtered)+2)
	lines = append(lines, file.Path)
	if message := strings.TrimSpace(file.Error); message != "" {
		lines = append(lines, "- diagnostics unavailable: "+message)
	}
	for _, diagnostic := range filtered {
		source := diagnostic.Source
		if strings.TrimSpace(source) == "" {
			source = "lsp"
		}
		lines = append(lines, fmt.Sprintf("- %d:%d [%s] %s (%s)", diagnostic.Line, diagnostic.Character, diagnostic.Severity, diagnostic.Message, source))
	}
	return strings.Join(lines, "\n")
}

func filterDiagnosticsForFile(filePath string, diagnostics []CodeIntelDiagnostic) []CodeIntelDiagnostic {
	maxLine := diagnosticsFileLineCount(filePath)
	if len(diagnostics) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]CodeIntelDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if maxLine > 0 && diagnostic.Line > maxLine {
			continue
		}
		key := fmt.Sprintf("%d:%d:%s:%s", diagnostic.Line, diagnostic.Character, diagnostic.Severity, diagnostic.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, diagnostic)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Character < out[j].Character
	})
	return out
}

func diagnosticsFileLineCount(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	return len(strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n"))
}

func formatSymbols(query string, result CodeIntelSymbolsResult) string {
	if message := formatCodeIntelNotice("symbols", result.Notice); message != "" {
		return message
	}
	if len(result.Symbols) == 0 {
		return fmt.Sprintf("No symbols found matching %q.", query)
	}
	lines := make([]string, 0, len(result.Symbols)+1)
	lines = append(lines, fmt.Sprintf("Symbols matching %q", query))
	for _, symbol := range result.Symbols {
		lines = append(lines, fmt.Sprintf("- %s [%s] %s:%d", symbol.Name, symbol.Kind, symbol.Path, symbol.Line))
	}
	return strings.Join(lines, "\n")
}

func formatTraceResult(request CodeIntelTraceRequest, result CodeIntelTraceResult) string {
	if message := formatCodeIntelNotice("trace", result.Notice); message != "" {
		return message
	}
	if !result.Supported {
		return "trace unsupported: this file type or language server does not support call hierarchy"
	}
	if !result.Found {
		return fmt.Sprintf("No callable symbol found at %s.", definitionTitle(request.Path, request.Line, request.Character))
	}
	nodeByID := make(map[string]CodeIntelTraceNode, len(result.Nodes))
	for _, node := range result.Nodes {
		nodeByID[node.ID] = node
	}
	root, ok := nodeByID[result.RootID]
	if !ok {
		return fmt.Sprintf("No callable symbol found at %s.", definitionTitle(request.Path, request.Line, request.Character))
	}
	switch request.Mode {
	case CodeIntelTraceModeCallers:
		return formatTraceCallList("Callers of ", "No callers found for ", root, traceEdgeNodes(result.Edges, nodeByID, func(edge CodeIntelTraceEdge) string {
			return edge.FromID
		}, result.RootID), result.Truncated, request.MaxNodes)
	case CodeIntelTraceModeCallees:
		return formatTraceCallList("Callees of ", "No callees found for ", root, traceEdgeNodes(result.Edges, nodeByID, func(edge CodeIntelTraceEdge) string {
			return edge.ToID
		}, result.RootID), result.Truncated, request.MaxNodes)
	default:
		return formatTraceGraph(root, result.Nodes, result.Edges, nodeByID, result.Truncated, request.MaxNodes, request.Depth)
	}
}

func formatRefsResult(request CodeIntelRefsRequest, result CodeIntelRefsResult) string {
	if message := formatCodeIntelNotice("refs", result.Notice); message != "" {
		return message
	}
	if !result.Supported {
		return "refs unsupported: this file type or language server does not support reference lookup"
	}
	targetLabel := formatRefsTarget(result.Target, request)
	if !result.Found || len(result.References) == 0 {
		switch request.Mode {
		case CodeIntelRefsModeReaders:
			lines := []string{"No readers found for " + targetLabel + "."}
			if !result.ClassificationSupported {
				lines = append(lines, "notice: read/write classification is not supported by this language server")
			} else if result.ClassificationIncomplete {
				lines = append(lines, "notice: some references could not be classified as readers or writers and were omitted")
			}
			return strings.Join(lines, "\n")
		case CodeIntelRefsModeWriters:
			lines := []string{"No writers found for " + targetLabel + "."}
			if !result.ClassificationSupported {
				lines = append(lines, "notice: read/write classification is not supported by this language server")
			} else if result.ClassificationIncomplete {
				lines = append(lines, "notice: some references could not be classified as readers or writers and were omitted")
			}
			return strings.Join(lines, "\n")
		default:
			return "No references found for " + targetLabel + "."
		}
	}

	lines := []string{}
	switch request.Mode {
	case CodeIntelRefsModeReaders:
		lines = append(lines, "Readers of "+targetLabel)
	case CodeIntelRefsModeWriters:
		lines = append(lines, "Writers of "+targetLabel)
	default:
		lines = append(lines, "References for "+targetLabel)
	}

	switch request.Mode {
	case CodeIntelRefsModeAll:
		appendGroupedRefs(&lines, "Readers", result.References, CodeIntelReferenceKindRead)
		appendGroupedRefs(&lines, "Writers", result.References, CodeIntelReferenceKindWrite)
		appendGroupedRefs(&lines, "Other References", result.References, CodeIntelReferenceKindReference)
	default:
		for _, ref := range result.References {
			lines = append(lines, "- "+formatReferenceLine(ref))
		}
	}
	if result.Truncated {
		lines = append(lines, fmt.Sprintf("notice: truncated at max_results=%d", request.MaxResults))
	}
	if request.Mode != CodeIntelRefsModeAll {
		if !result.ClassificationSupported {
			lines = append(lines, "notice: read/write classification is not supported by this language server")
		} else if result.ClassificationIncomplete {
			lines = append(lines, "notice: some references could not be classified as readers or writers and were omitted")
		}
	} else if result.ClassificationIncomplete {
		lines = append(lines, "notice: some references could not be classified as readers or writers")
	}
	return strings.Join(lines, "\n")
}

func appendGroupedRefs(lines *[]string, title string, refs []CodeIntelReference, kind CodeIntelReferenceKind) {
	group := make([]CodeIntelReference, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == kind {
			group = append(group, ref)
		}
	}
	if len(group) == 0 {
		return
	}
	*lines = append(*lines, title+":")
	for _, ref := range group {
		*lines = append(*lines, "- "+formatReferenceLine(ref))
	}
}

func formatReferenceLine(ref CodeIntelReference) string {
	base := fmt.Sprintf("%s:%d:%d", ref.Path, ref.Line, ref.Character)
	if strings.TrimSpace(ref.Snippet) == "" {
		return base
	}
	return base + ": " + ref.Snippet
}

func formatRefsTarget(target CodeIntelTraceNode, request CodeIntelRefsRequest) string {
	if strings.TrimSpace(target.Name) == "" {
		return definitionTitle(request.Path, request.Line, request.Character)
	}
	return fmt.Sprintf("%s [%s] %s:%d:%d", target.Name, target.Kind, target.Path, target.Line, target.Character)
}

func formatTraceCallList(prefix, emptyPrefix string, root CodeIntelTraceNode, related []CodeIntelTraceNode, truncated bool, maxNodes int) string {
	if len(related) == 0 {
		return emptyPrefix + formatTraceNode(root) + "."
	}
	lines := []string{prefix + formatTraceNode(root)}
	for _, node := range related {
		lines = append(lines, "- "+formatTraceNode(node))
	}
	if truncated {
		lines = append(lines, fmt.Sprintf("notice: truncated at max_nodes=%d", maxNodes))
	}
	return strings.Join(lines, "\n")
}

func formatTraceGraph(root CodeIntelTraceNode, nodes []CodeIntelTraceNode, edges []CodeIntelTraceEdge, nodeByID map[string]CodeIntelTraceNode, truncated bool, maxNodes, depth int) string {
	lines := []string{
		"Trace graph for " + formatTraceNode(root),
		fmt.Sprintf("depth: %d", depth),
		"Nodes:",
	}
	for _, node := range nodes {
		label := formatTraceNode(node)
		if node.ID == root.ID {
			label = "(target) " + label
		}
		lines = append(lines, "- "+label)
	}
	lines = append(lines, "Edges:")
	if len(edges) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, edge := range edges {
			from, okFrom := nodeByID[edge.FromID]
			to, okTo := nodeByID[edge.ToID]
			if !okFrom || !okTo {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s -> %s", formatTraceNodeRef(from), formatTraceNodeRef(to)))
		}
	}
	if truncated {
		lines = append(lines, fmt.Sprintf("notice: truncated at max_nodes=%d", maxNodes))
	}
	return strings.Join(lines, "\n")
}

func traceEdgeNodes(edges []CodeIntelTraceEdge, nodeByID map[string]CodeIntelTraceNode, pickID func(CodeIntelTraceEdge) string, excludeID string) []CodeIntelTraceNode {
	seen := make(map[string]bool)
	out := make([]CodeIntelTraceNode, 0, len(edges))
	for _, edge := range edges {
		id := pickID(edge)
		if id == "" || id == excludeID || seen[id] {
			continue
		}
		node, ok := nodeByID[id]
		if !ok {
			continue
		}
		seen[id] = true
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		return traceNodeLess(out[i], out[j])
	})
	return out
}

func formatTraceNode(node CodeIntelTraceNode) string {
	return fmt.Sprintf("%s [%s] %s:%d:%d", node.Name, node.Kind, node.Path, node.Line, node.Character)
}

func formatTraceNodeRef(node CodeIntelTraceNode) string {
	return fmt.Sprintf("%s %s:%d:%d", node.Name, node.Path, node.Line, node.Character)
}

func traceNodeLess(left, right CodeIntelTraceNode) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	if left.Character != right.Character {
		return left.Character < right.Character
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.Kind < right.Kind
}

func formatCodeIntelMutationSummary(actionLabel string, summary CodeIntelMutationSummary) string {
	if len(summary.Paths) == 0 {
		return "No file changes were produced."
	}
	output := fmt.Sprintf("%s across %d file(s) with %d text edit(s).", actionLabel, len(summary.Paths), summary.TextEdits)
	if summary.Created > 0 || summary.Renamed > 0 || summary.Deleted > 0 {
		output += fmt.Sprintf("\ncreated: %d, renamed: %d, deleted: %d", summary.Created, summary.Renamed, summary.Deleted)
	}
	return output + "\n" + strings.Join(summary.Paths, "\n")
}
