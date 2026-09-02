package main

import (
	"bytes"
	"encoding/json"
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
	if !root.IsObject() || hasDuplicateJSONMembers(body) {
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
	for _, path := range []string{
		"functionCall",
		"functionResponse",
		"function_call",
		"function_response",
		"inlineData",
		"inline_data",
		"fileData",
		"file_data",
		"executableCode",
		"executable_code",
		"codeExecutionResult",
		"code_execution_result",
		"thoughtSignature",
		"thought_signature",
		"functionCall.thoughtSignature",
		"functionCall.thought_signature",
		"functionResponse.thoughtSignature",
		"functionResponse.thought_signature",
		"extra_content.google.thought_signature",
	} {
		if part.Get(path).Exists() {
			return false
		}
	}
	return part.Get("thought").Type != gjson.True
}

func hasDuplicateJSONMembers(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	duplicate, err := jsonValueHasDuplicateMembers(decoder)
	return err != nil || duplicate
}

func jsonValueHasDuplicateMembers(decoder *json.Decoder) (bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return false, nil
	}

	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return false, errors.New("JSON object member name is not a string")
			}
			if _, exists := seen[key]; exists {
				return true, nil
			}
			seen[key] = struct{}{}
			if duplicate, err := jsonValueHasDuplicateMembers(decoder); err != nil || duplicate {
				return duplicate, err
			}
		}
		_, err = decoder.Token()
		return false, err
	case '[':
		for decoder.More() {
			if duplicate, err := jsonValueHasDuplicateMembers(decoder); err != nil || duplicate {
				return duplicate, err
			}
		}
		_, err = decoder.Token()
		return false, err
	default:
		return false, nil
	}
}
