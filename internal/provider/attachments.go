package provider

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func attachmentBase64Data(attachment Attachment) (string, error) {
	if err := attachment.Validate(); err != nil {
		return "", err
	}
	prefix := "data:" + strings.TrimSpace(attachment.MIMEType) + ";base64,"
	return strings.TrimPrefix(strings.TrimSpace(attachment.DataURL), prefix), nil
}

func attachmentBytes(attachment Attachment) ([]byte, error) {
	encoded, err := attachmentBase64Data(attachment)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode attachment %q: %w", attachment.Name, err)
	}
	return data, nil
}
