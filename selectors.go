package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

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
	if !root.IsObject() || hasDuplicateJSONMembers(root) {
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

func scanTextPart(part gjson.Result, requireTextType bool) (text gjson.Result, allowed bool) {
	if !part.IsObject() {
		return text, false
	}
	var partType gjson.Result
	typePresent := false
	machinePart := false
	thought := false
	part.ForEach(func(key, value gjson.Result) bool {
		switch key.Str {
		case "text":
			text = value
		case "type":
			typePresent = true
			partType = value
		case "thought":
			thought = value.Type == gjson.True
		case "functionCall", "functionResponse", "function_call", "function_response", "inlineData", "inline_data", "fileData", "file_data", "executableCode", "executable_code", "codeExecutionResult", "code_execution_result", "thoughtSignature", "thought_signature":
			machinePart = true
		case "extra_content":
			if value.Get("google.thought_signature").Exists() {
				machinePart = true
			}
		}
		return true
	})
	if machinePart || thought {
		return text, false
	}
	if requireTextType && typePresent && (partType.Type != gjson.String || partType.Str != "" && partType.Str != "text") {
		return text, false
	}
	return text, true
}

func hasDuplicateJSONMembers(root gjson.Result) bool {
	if root.IsObject() {
		seen := make(map[string]struct{})
		duplicate := false
		root.ForEach(func(key, value gjson.Result) bool {
			name, ok := canonicalJSONMemberName(key)
			if !ok {
				duplicate = true
				return false
			}
			if _, exists := seen[name]; exists {
				duplicate = true
				return false
			}
			seen[name] = struct{}{}
			if hasDuplicateJSONMembers(value) {
				duplicate = true
				return false
			}
			return true
		})
		return duplicate
	}
	if root.IsArray() {
		duplicate := false
		root.ForEach(func(_, value gjson.Result) bool {
			if hasDuplicateJSONMembers(value) {
				duplicate = true
				return false
			}
			return true
		})
		return duplicate
	}
	return false
}

func canonicalJSONMemberName(key gjson.Result) (string, bool) {
	if !strings.ContainsRune(key.Raw, '\\') && utf8.ValidString(key.Str) {
		return key.Str, true
	}
	var decoded string
	if err := json.Unmarshal([]byte(key.Raw), &decoded); err != nil {
		return "", false
	}
	return decoded, true
}
