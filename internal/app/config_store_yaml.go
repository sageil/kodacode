package app

import (
	"strings"

	"gopkg.in/yaml.v3"
)

func ensureDocumentMapping(document *yaml.Node) *yaml.Node {
	if document == nil {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if document.Kind != yaml.DocumentNode {
		document.Kind = yaml.DocumentNode
	}
	if len(document.Content) == 0 || document.Content[0] == nil {
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
		return document.Content[0]
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		root.Kind = yaml.MappingNode
		root.Tag = "!!map"
		root.Content = nil
	}
	return root
}

func ensureMappingValue(root *yaml.Node, key string) *yaml.Node {
	if existing := mappingLookup(root, key); existing != nil && existing.Kind == yaml.MappingNode {
		return existing
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMappingNode(root, key, node)
	return node
}

func ensureSequenceValue(root *yaml.Node, key string) *yaml.Node {
	if existing := mappingLookup(root, key); existing != nil && existing.Kind == yaml.SequenceNode {
		return existing
	}
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	setMappingNode(root, key, node)
	return node
}

func providerNodeByID(sequence *yaml.Node, providerID string) *yaml.Node {
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return nil
	}
	for _, entry := range sequence.Content {
		if providerIDFromNode(entry) == providerID {
			return entry
		}
	}
	return nil
}

func providerIDFromNode(entry *yaml.Node) string {
	if entry == nil || entry.Kind != yaml.MappingNode {
		return ""
	}
	return strings.TrimSpace(scalarValue(mappingLookup(entry, "id")))
}

func newProviderMappingNode(providerID string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			newScalarNode("id"),
			newScalarNode(providerID),
			newScalarNode("base_url"),
			newScalarNode(""),
		},
	}
}

func setMappingScalar(root *yaml.Node, key, value string) {
	setMappingNode(root, key, newScalarNode(value))
}

func setMappingNode(root *yaml.Node, key string, value *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		if root.Content[idx].Value == key {
			root.Content[idx+1] = value
			return
		}
	}
	root.Content = append(root.Content, newScalarNode(key), value)
}

func mappingLookup(root *yaml.Node, key string) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		if root.Content[idx].Value == key {
			return root.Content[idx+1]
		}
	}
	return nil
}

func removeMappingKey(root *yaml.Node, key string) bool {
	if root == nil || root.Kind != yaml.MappingNode {
		return false
	}
	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		if root.Content[idx].Value != key {
			continue
		}
		root.Content = append(root.Content[:idx], root.Content[idx+2:]...)
		return true
	}
	return false
}

func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func newScalarNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
}

func newIntNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!int",
		Value: value,
	}
}

func removeProviderAPIKeys(root *yaml.Node) bool {
	if root == nil || root.Kind != yaml.MappingNode {
		return false
	}

	changed := false
	if providers := mappingLookup(root, "providers"); providers != nil && providers.Kind == yaml.SequenceNode {
		for _, entry := range providers.Content {
			if removeMappingKey(entry, "api_key") {
				changed = true
			}
		}
	}
	return changed
}
