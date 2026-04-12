// google_export_test.go — test-only exports for white-box testing of google_convert.go.
package provider

import "google.golang.org/genai"

// ExportConvertGoogleMessages exposes convertGoogleMessages for tests.
func ExportConvertGoogleMessages(msgs []Message) ([]*genai.Content, error) {
	return convertGoogleMessages(msgs, false)
}

// ExportBuildGoogleConfig exposes buildGoogleConfig for tests.
func ExportBuildGoogleConfig(opts ChatOptions) (*genai.GenerateContentConfig, error) {
	return buildGoogleConfig(opts, false, true)
}

// ExportNormalizeGoogleFinishReason exposes normalizeGoogleFinishReason for tests.
func ExportNormalizeGoogleFinishReason(reason string, hasTools bool) string {
	return normalizeGoogleFinishReason(reason, hasTools)
}

// ExportNextGoogleCallID exposes the call ID generator for tests.
func ExportNextGoogleCallID() string {
	return nextGoogleCallID()
}
