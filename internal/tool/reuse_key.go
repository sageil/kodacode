package tool

import "encoding/json"

func canonicalToolArgsKey(args json.RawMessage) (string, error) {
	var decoded any
	if err := json.Unmarshal(args, &decoded); err != nil {
		return "", err
	}
	key, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(key), nil
}
