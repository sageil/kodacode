package app

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type unsupportedConfigKeysError struct {
	keys []string
}

func (e unsupportedConfigKeysError) Error() string {
	switch len(e.keys) {
	case 0:
		return "unsupported config keys"
	case 1:
		return fmt.Sprintf("unsupported config key %q; use %s", e.keys[0], replacementForConfigKey(e.keys[0]))
	default:
		parts := make([]string, 0, len(e.keys))
		for _, key := range e.keys {
			parts = append(parts, fmt.Sprintf("%q -> %s", key, replacementForConfigKey(key)))
		}
		return "unsupported config keys: " + strings.Join(parts, ", ")
	}
}

func decodeStoredConfigDocument(data []byte) (StoredConfig, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return StoredConfig{}, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return StoredConfig{}, err
	}
	root := ensureDocumentMapping(&document)
	if err := rejectUnsupportedTopLevelConfigKeys(root); err != nil {
		return StoredConfig{}, err
	}
	var cfg StoredConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return StoredConfig{}, err
	}
	return cfg, nil
}

func rejectUnsupportedTopLevelConfigKeys(root *yaml.Node) error {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	unsupported := make([]string, 0, len(root.Content)/2)
	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		key := strings.TrimSpace(root.Content[idx].Value)
		if replacementForConfigKey(key) == "" {
			continue
		}
		unsupported = append(unsupported, key)
	}
	if len(unsupported) == 0 {
		return nil
	}
	sort.Strings(unsupported)
	return unsupportedConfigKeysError{keys: unsupported}
}

func replacementForConfigKey(key string) string {
	switch strings.TrimSpace(key) {
	case "session":
		return "sessions"
	case "debug":
		return "logging.debug"
	case "model_refresh_interval":
		return "model_cache.expiry_days"
	default:
		return ""
	}
}
