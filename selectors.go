package main

import (
	"bytes"
	"errors"

	"github.com/tidwall/gjson"
)

var (
	errInvalidRequest = errors.New("request body must be a JSON object")
	errInvalidSpan    = errors.New("selector returned an invalid span")
)

func knownSourceFormat(sourceFormat string) bool {
	switch sourceFormat {
	case "openai", "openai-response", "claude", "gemini", "interactions":
		return true
	default:
		return false
	}
}

func selectTextSpans(body []byte, sourceFormat string, roles scopeSet) ([]textSpan, error) {
	if !gjson.ValidBytes(body) || len(bytes.TrimSpace(body)) == 0 {
		return nil, errInvalidRequest
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return nil, errInvalidRequest
	}
	var spans []textSpan
	switch sourceFormat {
	case "openai":
		collectOpenAI(root, roles, &spans)
	case "openai-response":
		collectOpenAIResponses(root, roles, &spans)
	case "claude":
		collectClaude(root, roles, &spans)
	case "gemini":
		collectGemini(root, roles, &spans)
	case "interactions":
		collectInteractions(root, roles, &spans)
	}
	return spans, nil
}

func appendStringSpan(*[]textSpan, gjson.Result, string, scopeSet) {}

func geminiTextPartAllowed(part gjson.Result) bool {
	for _, key := range []string{
		"functionCall",
		"functionResponse",
		"inlineData",
		"inline_data",
		"fileData",
		"file_data",
		"executableCode",
		"codeExecutionResult",
	} {
		if part.Get(key).Exists() {
			return false
		}
	}
	if part.Get("thought").Type == gjson.True || part.Get("thoughtSignature").Exists() {
		return false
	}
	return true
}
