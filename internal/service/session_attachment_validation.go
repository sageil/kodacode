package service

import (
	"fmt"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func validateAttachmentsForProvider(prov provider.Provider, model provider.Model, attachments []FileAttachment) error {
	if len(attachments) == 0 {
		return nil
	}

	supporter, ok := prov.(provider.AttachmentSupporter)
	if !ok {
		return nil
	}

	caps := supporter.AttachmentCapabilities(model).ForModel(model)
	for _, att := range attachments {
		if caps.Supports(att.MimeType) {
			continue
		}
		return fmt.Errorf("send: provider %q model %q does not support %s attachments", prov.ID(), model.ID, att.MimeType)
	}
	return nil
}
