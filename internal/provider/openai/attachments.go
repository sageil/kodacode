package openai

import (
	"encoding/base64"
	"path/filepath"
	"strings"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/responses"
	"github.com/sageil/kodacode/v1/internal/provider"
)

func openAIResponseContentPart(part provider.MessagePart) (responses.ResponseInputContentUnionParam, bool) {
	switch p := part.(type) {
	case provider.TextPart:
		return responses.ResponseInputContentUnionParam{
			OfInputText: &responses.ResponseInputTextParam{Text: p.Text},
		}, true
	case provider.FilePart:
		if provider.IsImageMIME(p.MimeType) {
			return responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					Detail:   responses.ResponseInputImageDetailAuto,
					ImageURL: param.NewOpt(p.URL),
				},
			}, true
		}
		return responses.ResponseInputContentUnionParam{
			OfInputFile: &responses.ResponseInputFileParam{
				FileData: param.NewOpt(openAIFileData(p)),
				Filename: param.NewOpt(openAIFilename(p)),
			},
		}, true
	}
	return responses.ResponseInputContentUnionParam{}, false
}

func openAIChatContentPart(part provider.MessagePart) (openaisdk.ChatCompletionContentPartUnionParam, bool) {
	switch p := part.(type) {
	case provider.TextPart:
		return openaisdk.TextContentPart(p.Text), true
	case provider.FilePart:
		if provider.IsImageMIME(p.MimeType) {
			return openaisdk.ImageContentPart(openaisdk.ChatCompletionContentPartImageImageURLParam{
				URL: p.URL,
			}), true
		}
		return openaisdk.FileContentPart(openaisdk.ChatCompletionContentPartFileFileParam{
			FileData: param.NewOpt(openAIFileData(p)),
			Filename: param.NewOpt(openAIFilename(p)),
		}), true
	}
	return openaisdk.ChatCompletionContentPartUnionParam{}, false
}

func openAIFileData(part provider.FilePart) string {
	if strings.HasPrefix(part.URL, "data:") {
		return provider.ExtractBase64Data(part.URL)
	}
	return base64.StdEncoding.EncodeToString([]byte(part.URL))
}

func openAIFilename(part provider.FilePart) string {
	name := filepath.Base(part.Path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "attachment"
	}
	return name
}
