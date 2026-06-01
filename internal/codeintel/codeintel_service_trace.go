package codeintel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/lsp"
	"github.com/sageil/kodacode/internal/tool"
)

type callHierarchyServer interface {
	PrepareCallHierarchy(ctx context.Context, filePath string, line, character int) ([]lsp.CallHierarchyItem, error)
	IncomingCalls(ctx context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyIncomingCall, error)
	OutgoingCalls(ctx context.Context, item lsp.CallHierarchyItem) ([]lsp.CallHierarchyOutgoingCall, error)
}

func (w *workspaceCodeIntel) Trace(ctx context.Context, request tool.CodeIntelTraceRequest) (tool.CodeIntelTraceResult, error) {
	manager := w.service.managerForPath(w.roots, request.Path)
	if manager == nil {
		return tool.CodeIntelTraceResult{
			Notice: codeIntelUnavailableNotice(errors.New("no code-intelligence workspace root matched this path")),
		}, nil
	}
	server, err := manager.ServerForPath(ctx, request.Path)
	if err != nil {
		if errors.Is(err, lsp.ErrServerNotConfigured) {
			return tool.CodeIntelTraceResult{Notice: codeIntelUnavailableNotice(err)}, nil
		}
		return tool.CodeIntelTraceResult{Notice: codeIntelUnavailableNotice(err)}, nil
	}
	return traceFromServer(ctx, server, request)
}

func traceFromServer(ctx context.Context, server callHierarchyServer, request tool.CodeIntelTraceRequest) (tool.CodeIntelTraceResult, error) {
	var (
		items []lsp.CallHierarchyItem
		err   error
	)
	for _, candidate := range tool.BestEffortPositionCandidates(request.Path, request.Line, request.Character) {
		items, err = server.PrepareCallHierarchy(ctx, request.Path, request.Line-1, candidate)
		if err != nil {
			if errors.Is(err, lsp.ErrCallHierarchyUnsupported) {
				return tool.CodeIntelTraceResult{
					Notice: codeIntelUnsupportedNotice("this file type or language server does not support call hierarchy"),
				}, nil
			}
			return tool.CodeIntelTraceResult{}, err
		}
		if len(items) > 0 {
			break
		}
	}
	if len(items) == 0 {
		return tool.CodeIntelTraceResult{
			Supported: true,
			Found:     false,
		}, nil
	}
	root := selectTraceRootItem(items)
	switch request.Mode {
	case tool.CodeIntelTraceModeCallers:
		return traceCallers(ctx, server, root, request.MaxNodes)
	case tool.CodeIntelTraceModeCallees:
		return traceCallees(ctx, server, root, request.MaxNodes)
	default:
		return traceGraph(ctx, server, root, request.Depth, request.MaxNodes)
	}
}

func traceCallers(ctx context.Context, server callHierarchyServer, root lsp.CallHierarchyItem, maxNodes int) (tool.CodeIntelTraceResult, error) {
	calls, err := server.IncomingCalls(ctx, root)
	if err != nil {
		if errors.Is(err, lsp.ErrCallHierarchyUnsupported) {
			return tool.CodeIntelTraceResult{
				Notice: codeIntelUnsupportedNotice("this file type or language server does not support call hierarchy"),
			}, nil
		}
		return tool.CodeIntelTraceResult{}, err
	}
	sort.Slice(calls, func(i, j int) bool {
		return traceItemLess(calls[i].From, calls[j].From)
	})
	collector := newTraceCollector(maxNodes)
	rootNode := traceNodeFromItem(root)
	collector.ensureNode(rootNode)
	for _, call := range calls {
		fromNode := traceNodeFromItem(call.From)
		if !collector.ensureNode(fromNode) {
			continue
		}
		collector.addEdge(fromNode.ID, rootNode.ID)
	}
	return collector.result(rootNode), nil
}

func traceCallees(ctx context.Context, server callHierarchyServer, root lsp.CallHierarchyItem, maxNodes int) (tool.CodeIntelTraceResult, error) {
	calls, err := server.OutgoingCalls(ctx, root)
	if err != nil {
		if errors.Is(err, lsp.ErrCallHierarchyUnsupported) {
			return tool.CodeIntelTraceResult{
				Notice: codeIntelUnsupportedNotice("this file type or language server does not support call hierarchy"),
			}, nil
		}
		return tool.CodeIntelTraceResult{}, err
	}
	sort.Slice(calls, func(i, j int) bool {
		return traceItemLess(calls[i].To, calls[j].To)
	})
	collector := newTraceCollector(maxNodes)
	rootNode := traceNodeFromItem(root)
	collector.ensureNode(rootNode)
	for _, call := range calls {
		toNode := traceNodeFromItem(call.To)
		if !collector.ensureNode(toNode) {
			continue
		}
		collector.addEdge(rootNode.ID, toNode.ID)
	}
	return collector.result(rootNode), nil
}

func traceGraph(ctx context.Context, server callHierarchyServer, root lsp.CallHierarchyItem, depth, maxNodes int) (tool.CodeIntelTraceResult, error) {
	if depth < 1 {
		depth = 1
	}
	rootNode := traceNodeFromItem(root)
	collector := newTraceCollector(maxNodes)
	collector.ensureNode(rootNode)

	type queuedItem struct {
		item  lsp.CallHierarchyItem
		depth int
	}
	queue := []queuedItem{{item: root, depth: 0}}
	expanded := map[string]bool{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		currentKey := traceNodeKey(current.item)
		if expanded[currentKey] || current.depth >= depth {
			continue
		}
		expanded[currentKey] = true
		currentNode := traceNodeFromItem(current.item)

		incoming, err := server.IncomingCalls(ctx, current.item)
		if err != nil {
			if errors.Is(err, lsp.ErrCallHierarchyUnsupported) {
				return tool.CodeIntelTraceResult{
					Notice: codeIntelUnsupportedNotice("this file type or language server does not support call hierarchy"),
				}, nil
			}
			return tool.CodeIntelTraceResult{}, err
		}
		sort.Slice(incoming, func(i, j int) bool {
			return traceItemLess(incoming[i].From, incoming[j].From)
		})
		for _, call := range incoming {
			fromNode := traceNodeFromItem(call.From)
			if !collector.ensureNode(fromNode) {
				continue
			}
			collector.addEdge(fromNode.ID, currentNode.ID)
			if current.depth+1 < depth && !expanded[fromNode.ID] {
				queue = append(queue, queuedItem{item: call.From, depth: current.depth + 1})
			}
		}

		outgoing, err := server.OutgoingCalls(ctx, current.item)
		if err != nil {
			if errors.Is(err, lsp.ErrCallHierarchyUnsupported) {
				return tool.CodeIntelTraceResult{
					Notice: codeIntelUnsupportedNotice("this file type or language server does not support call hierarchy"),
				}, nil
			}
			return tool.CodeIntelTraceResult{}, err
		}
		sort.Slice(outgoing, func(i, j int) bool {
			return traceItemLess(outgoing[i].To, outgoing[j].To)
		})
		for _, call := range outgoing {
			toNode := traceNodeFromItem(call.To)
			if !collector.ensureNode(toNode) {
				continue
			}
			collector.addEdge(currentNode.ID, toNode.ID)
			if current.depth+1 < depth && !expanded[toNode.ID] {
				queue = append(queue, queuedItem{item: call.To, depth: current.depth + 1})
			}
		}
	}

	return collector.result(rootNode), nil
}

type traceCollector struct {
	maxNodes  int
	truncated bool
	nodes     map[string]tool.CodeIntelTraceNode
	edges     map[string]tool.CodeIntelTraceEdge
}

func newTraceCollector(maxNodes int) *traceCollector {
	return &traceCollector{
		maxNodes: maxNodes,
		nodes:    make(map[string]tool.CodeIntelTraceNode),
		edges:    make(map[string]tool.CodeIntelTraceEdge),
	}
}

func (c *traceCollector) ensureNode(node tool.CodeIntelTraceNode) bool {
	if _, ok := c.nodes[node.ID]; ok {
		return true
	}
	if c.maxNodes > 0 && len(c.nodes) >= c.maxNodes {
		c.truncated = true
		return false
	}
	c.nodes[node.ID] = node
	return true
}

func (c *traceCollector) addEdge(fromID, toID string) {
	if fromID == "" || toID == "" {
		return
	}
	key := fromID + "\x00" + toID
	c.edges[key] = tool.CodeIntelTraceEdge{FromID: fromID, ToID: toID}
}

func (c *traceCollector) result(root tool.CodeIntelTraceNode) tool.CodeIntelTraceResult {
	nodes := make([]tool.CodeIntelTraceNode, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return traceNodeLess(nodes[i], nodes[j])
	})

	edges := make([]tool.CodeIntelTraceEdge, 0, len(c.edges))
	for _, edge := range c.edges {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromID != edges[j].FromID {
			return edges[i].FromID < edges[j].FromID
		}
		return edges[i].ToID < edges[j].ToID
	})

	return tool.CodeIntelTraceResult{
		Supported: true,
		Found:     true,
		RootID:    root.ID,
		Nodes:     nodes,
		Edges:     edges,
		Truncated: c.truncated,
	}
}

func selectTraceRootItem(items []lsp.CallHierarchyItem) lsp.CallHierarchyItem {
	sorted := append([]lsp.CallHierarchyItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		return traceItemLess(sorted[i], sorted[j])
	})
	return sorted[0]
}

func traceNodeFromItem(item lsp.CallHierarchyItem) tool.CodeIntelTraceNode {
	path := lsp.URIToPath(item.URI)
	name := strings.TrimSpace(item.Name)
	return tool.CodeIntelTraceNode{
		ID:        fmt.Sprintf("%s\x00%d\x00%d\x00%s", path, item.SelectionRange.Start.Line+1, item.SelectionRange.Start.Character, name),
		Name:      name,
		Kind:      lsp.SymbolKindName(item.Kind),
		Path:      path,
		Line:      item.SelectionRange.Start.Line + 1,
		Character: item.SelectionRange.Start.Character,
	}
}

func traceNodeKey(item lsp.CallHierarchyItem) string {
	return traceNodeFromItem(item).ID
}

func traceItemLess(left, right lsp.CallHierarchyItem) bool {
	return traceNodeLess(traceNodeFromItem(left), traceNodeFromItem(right))
}

func traceNodeLess(left, right tool.CodeIntelTraceNode) bool {
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
