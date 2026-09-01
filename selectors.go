package main

import (
	"bytes"
	"errors"
	"sort"

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
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].RawStart < spans[j].RawStart
	})
	for i, span := range spans {
		if span.RawStart < 0 || span.RawStart >= span.RawEnd || span.RawEnd > len(body) {
			return nil, errInvalidSpan
		}
		if i > 0 && spans[i-1].RawEnd > span.RawStart {
			return nil, errInvalidSpan
		}
	}
	return spans, nil
}

func appendStringSpan(spans *[]textSpan, value gjson.Result, role string, roles scopeSet) {
	if value.Type != gjson.String || !roles.has(role) || len(value.Raw) == 0 || value.Index < 0 || value.Index > int(^uint(0)>>1)-len(value.Raw) {
		return
	}
	*spans = append(*spans, textSpan{
		RawStart: value.Index,
		RawEnd:   value.Index + len(value.Raw),
		Text:     value.Str,
		Role:     role,
	})
}

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
