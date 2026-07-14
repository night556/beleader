package api

import (
	"github.com/sashabaranov/go-openai"
)

func buildTextAndImageMultiContent(text string, images []string) []openai.ChatMessagePart {
	var parts []openai.ChatMessagePart
	if text != "" {
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeText,
			Text: text,
		})
	}
	for _, img := range images {
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: img,
			},
		})
	}
	return parts
}
