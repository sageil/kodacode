package provider

import "strings"

// AttachmentCapabilities describes which attachment classes a provider can
// accept for a given model.
type AttachmentCapabilities struct {
	Images bool
	PDFs   bool
	Text   bool
	Binary bool
}

func (c AttachmentCapabilities) ForModel(model Model) AttachmentCapabilities {
	if model.AttachmentKnown && !model.Attachment {
		c.PDFs = false
		c.Text = false
		c.Binary = false
	}
	if model.VisionKnown && !model.Vision {
		c.Images = false
	}
	if model.AttachmentKnown && !model.Attachment && !model.Vision {
		c.Images = false
	}
	return c
}

func (c AttachmentCapabilities) Supports(mime string) bool {
	switch {
	case IsImageMIME(mime):
		return c.Images
	case mime == "application/pdf":
		return c.PDFs
	case isTextLikeAttachmentMIME(mime):
		return c.Text
	default:
		return c.Binary
	}
}

// AttachmentSupporter is an optional interface a provider may implement to
// declare attachment support up front so callers can fail before dispatch.
type AttachmentSupporter interface {
	AttachmentCapabilities(model Model) AttachmentCapabilities
}

func isTextLikeAttachmentMIME(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml", "application/javascript":
		return true
	default:
		return false
	}
}
