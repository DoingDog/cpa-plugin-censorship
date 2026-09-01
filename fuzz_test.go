package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

const maxFuzzJSONDepth = 256

type fuzzBlock struct {
	Term string
	Role string
}

type fuzzRuleResult struct {
	Texts   []string
	Blocked *fuzzBlock
}

type oracleTextMatch struct {
	Start int
	End   int
}

func FuzzRuleEngineAgainstOracle(f *testing.F) {
	for _, seed := range []struct {
		text, terms string
		mode, fold  uint8
	}{
		{"ababa", "aba|ba", 0, 0},
		{"ALPHA Alpha", "Alpha", 1, 1},
		{"ςΣσ", "Σ", 2, 1},
		{"STRASSE/straße", "straße", 1, 1},
		{"世界世界", "世界", 2, 0},
	} {
		f.Add(seed.text, seed.terms, seed.mode, seed.fold)
	}
	f.Fuzz(func(t *testing.T, text, packed string, modeByte, foldByte uint8) {
		terms := boundedTerms(packed, 8, 32)
		if len(terms) == 0 || len(text) > 256 {
			t.Skip()
		}
		cfg := snapshotForFuzz(terms, modeByte%3, foldByte%2 == 1)
		if cfg == nil {
			t.Skip()
		}
		got := applyRulesToTextsForTest([]string{text, "prefix " + text}, cfg)
		want := oracleApply([]string{text, "prefix " + text}, cfg)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})
}

func FuzzProtocolTransform(f *testing.F) {
	seeds := []struct {
		format string
		body   []byte
	}{
		{format: "openai", body: []byte(`{"messages":[{"role":"user","content":"SECRET"}],"tools":[{"description":"SECRET"}]}`)},
		{format: "openai-response", body: []byte(`{"instructions":"SECRET","input":[{"type":"function_call_output","output":"SECRET"}]}`)},
		{format: "claude", body: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"},{"type":"thinking","thinking":"SECRET"}]}]}`)},
		{format: "gemini", body: []byte(`{"contents":[{"role":"user","parts":[{"text":"SECRET"},{"text":"SECRET","inlineData":{"data":"SECRET"}}]}]}`)},
		{format: "interactions", body: interactionStepsSeed(64)},
		{format: "openai", body: []byte(`not-json`)},
	}
	for _, seed := range seeds {
		for mode := uint8(0); mode < 3; mode++ {
			f.Add(seed.format, seed.body, mode, false)
			f.Add(seed.format, seed.body, mode, true)
		}
	}
	f.Fuzz(func(t *testing.T, format string, body []byte, modeByte uint8, fold bool) {
		if len(format) > 32 || len(body) > 64<<10 {
			t.Skip()
		}
		if json.Valid(body) && !boundedJSONNesting(body, maxFuzzJSONDepth) {
			t.Skip()
		}
		modes := []string{"block", "strip", "obfs"}
		cfg := mustConfig(t, fmt.Sprintf("mode: %s\nignore_case: %t\nwords: [SECRET]\n", modes[modeByte%3], fold))
		beforeExcluded := oracleExcludedTokens(format, body)
		got, err := transformRequest(body, format, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Invalid {
			if validJSONObject(body) && knownFormat(format) {
				t.Fatalf("valid JSON object marked invalid: %s", body)
			}
			return
		}
		if got.Blocked != nil {
			if got.Blocked.Term != "SECRET" || !allowedCanonicalRole(got.Blocked.Role) {
				t.Fatalf("blocked = %#v", got.Blocked)
			}
			return
		}
		if len(got.Body) == 0 {
			return
		}
		if !json.Valid(got.Body) {
			t.Fatalf("changed output is invalid JSON: %s", got.Body)
		}
		if after := oracleExcludedTokens(format, got.Body); !reflect.DeepEqual(after, beforeExcluded) {
			t.Fatalf("excluded raw tokens changed: before=%q after=%q", beforeExcluded, after)
		}
	})
}

func interactionStepsSeed(depth int) []byte {
	node := `{"content":"SECRET"}`
	for i := 0; i < depth; i++ {
		node = `{"steps":[` + node + `]}`
	}
	return []byte(`{"input":[` + node + `]}`)
}

func boundedTerms(packed string, maxTerms, maxScalars int) []string {
	if maxTerms <= 0 || maxScalars <= 0 || len(packed) > maxTerms*(maxScalars*utf8.UTFMax+1) {
		return nil
	}
	terms := strings.Split(packed, "|")
	if len(terms) > maxTerms {
		return nil
	}
	for _, term := range terms {
		if term == "" || utf8.RuneCountInString(term) > maxScalars {
			return nil
		}
	}
	return terms
}

func snapshotForFuzz(terms []string, modeByte uint8, fold bool) *configSnapshot {
	modes := [...]mode{modeBlock, modeStrip, modeObfs}
	selected := modes[modeByte%uint8(len(modes))]
	if selected == modeObfs {
		for _, term := range terms {
			if utf8.RuneCountInString(term) < 2 || strings.Contains(term, "​") {
				return nil
			}
		}
	}
	rules := make([]compiledRule, len(terms))
	for i, term := range terms {
		rules[i] = compiledRule{Term: term, Runes: []rune(term)}
	}
	return &configSnapshot{
		Mode:       selected,
		IgnoreCase: fold,
		Rules:      rules,
		ObfsChar:   "​",
	}
}

func fuzzRole(index int) string {
	roles := [...]string{"user", "developer", "assistant", "system"}
	return roles[index%len(roles)]
}

func applyRulesToTextsForTest(texts []string, cfg *configSnapshot) fuzzRuleResult {
	spans := make([]textSpan, len(texts))
	for i, text := range texts {
		spans[i] = textSpan{Text: text, Role: fuzzRole(i)}
	}
	blocked, _ := applyMode(spans, cfg)
	result := fuzzRuleResult{Texts: make([]string, len(spans))}
	for i := range spans {
		result.Texts[i] = spans[i].Text
	}
	if blocked != nil {
		result.Blocked = &fuzzBlock{Term: blocked.Term, Role: blocked.Role}
	}
	return result
}

func oracleApply(texts []string, cfg *configSnapshot) fuzzRuleResult {
	result := fuzzRuleResult{Texts: append([]string(nil), texts...)}
	switch cfg.Mode {
	case modeBlock:
		for _, rule := range cfg.Rules {
			for i, text := range result.Texts {
				if oracleContains(text, rule.Term, cfg.IgnoreCase) {
					result.Blocked = &fuzzBlock{Term: rule.Term, Role: fuzzRole(i)}
					return result
				}
			}
		}
	case modeStrip:
		for _, rule := range cfg.Rules {
			for i, text := range result.Texts {
				result.Texts[i] = oracleStrip(text, rule.Term, cfg.IgnoreCase)
			}
		}
	case modeObfs:
		for _, rule := range cfg.Rules {
			for i, text := range result.Texts {
				result.Texts[i] = oracleObfuscate(text, rule.Term, cfg.IgnoreCase, cfg.ObfsChar)
			}
		}
	}
	return result
}

func oracleContains(text, term string, fold bool) bool {
	if !fold {
		return strings.Contains(text, term)
	}
	return len(oracleFoldMatches(text, term)) != 0
}

func oracleStrip(text, term string, fold bool) string {
	matches := oracleMatches(text, term, fold)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	position := 0
	for _, match := range matches {
		out.WriteString(text[position:match.Start])
		position = match.End
	}
	out.WriteString(text[position:])
	return out.String()
}

func oracleObfuscate(text, term string, fold bool, char string) string {
	matches := oracleMatches(text, term, fold)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + len(matches)*len(char))
	position := 0
	for _, match := range matches {
		out.WriteString(text[position:match.Start])
		_, firstSize := utf8.DecodeRuneInString(text[match.Start:match.End])
		insertion := match.Start + firstSize
		out.WriteString(text[match.Start:insertion])
		out.WriteString(char)
		out.WriteString(text[insertion:match.End])
		position = match.End
	}
	out.WriteString(text[position:])
	return out.String()
}

func oracleMatches(text, term string, fold bool) []oracleTextMatch {
	if fold {
		return oracleFoldMatches(text, term)
	}
	return oracleLiteralMatches(text, term)
}

func oracleLiteralMatches(text, term string) []oracleTextMatch {
	if term == "" {
		return nil
	}
	var matches []oracleTextMatch
	for position := 0; position <= len(text); {
		index := strings.Index(text[position:], term)
		if index < 0 {
			break
		}
		start := position + index
		end := start + len(term)
		matches = append(matches, oracleTextMatch{Start: start, End: end})
		position = end
	}
	return matches
}

func oracleFoldMatches(text, term string) []oracleTextMatch {
	termScalars := utf8.RuneCountInString(term)
	if termScalars == 0 {
		return nil
	}
	offsets := make([]int, 0, utf8.RuneCountInString(text)+1)
	for offset := range text {
		offsets = append(offsets, offset)
	}
	offsets = append(offsets, len(text))

	var matches []oracleTextMatch
	for index := 0; index+termScalars < len(offsets); {
		start, end := offsets[index], offsets[index+termScalars]
		window := []rune(text[start:end])
		if len(window) == termScalars && strings.EqualFold(string(window), term) {
			matches = append(matches, oracleTextMatch{Start: start, End: end})
			index += termScalars
		} else {
			index++
		}
	}
	return matches
}

func knownFormat(format string) bool {
	switch format {
	case "openai", "openai-response", "claude", "gemini", "interactions":
		return true
	default:
		return false
	}
}

func validJSONObject(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) != 0 && trimmed[0] == '{' && json.Valid(trimmed)
}

func allowedCanonicalRole(role string) bool {
	switch role {
	case "system", "developer", "user", "assistant", "tool":
		return true
	default:
		return false
	}
}

func boundedJSONNesting(body []byte, maxDepth int) bool {
	depth := 0
	inString := false
	escaped := false
	for _, b := range body {
		if inString {
			switch {
			case escaped:
				escaped = false
			case b == '\\':
				escaped = true
			case b == '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > maxDepth {
				return false
			}
		case '}', ']':
			depth--
		}
	}
	return depth == 0 && !inString
}

func oracleExcludedTokens(format string, body []byte) [][]byte {
	if !validJSONObject(body) || !boundedJSONNesting(body, maxFuzzJSONDepth) {
		return nil
	}
	var tokens [][]byte
	oracleForEachObject(body, func(key string, value json.RawMessage) {
		switch format {
		case "openai":
			if key == "tools" {
				oracleForEachArray(value, func(item json.RawMessage) {
					appendOracleToken(&tokens, item)
				})
			}
		case "openai-response":
			if key == "input" {
				oracleForEachArray(value, func(item json.RawMessage) {
					itemType, ok := oracleUniqueStringField(item, "type")
					if ok && (itemType == "function_call_output" || itemType == "custom_tool_call_output") {
						appendOracleToken(&tokens, item)
					}
				})
			}
		case "claude":
			if key == "messages" {
				oracleClaudeExcluded(value, &tokens)
			}
		case "gemini":
			switch key {
			case "systemInstruction", "system_instruction":
				oracleForEachObject(value, func(innerKey string, innerValue json.RawMessage) {
					if innerKey == "parts" {
						oracleGeminiExcludedParts(innerValue, &tokens)
					}
				})
			case "contents":
				oracleForEachArray(value, func(content json.RawMessage) {
					oracleForEachObject(content, func(innerKey string, innerValue json.RawMessage) {
						if innerKey == "parts" {
							oracleGeminiExcludedParts(innerValue, &tokens)
						}
					})
				})
			}
		}
	})
	return tokens
}

func oracleClaudeExcluded(messages json.RawMessage, tokens *[][]byte) {
	oracleForEachArray(messages, func(message json.RawMessage) {
		oracleForEachObject(message, func(key string, value json.RawMessage) {
			if key != "content" {
				return
			}
			oracleForEachArray(value, func(block json.RawMessage) {
				blockType, ok := oracleUniqueStringField(block, "type")
				if ok && (blockType == "thinking" || blockType == "redacted_thinking" || blockType == "tool_use") {
					appendOracleToken(tokens, block)
				}
			})
		})
	})
}

func oracleGeminiExcludedParts(parts json.RawMessage, tokens *[][]byte) {
	oracleForEachArray(parts, func(part json.RawMessage) {
		if oracleGeminiPartExcluded(part) {
			appendOracleToken(tokens, part)
		}
	})
}

func oracleGeminiPartExcluded(part json.RawMessage) bool {
	excluded := false
	oracleForEachObject(part, func(key string, value json.RawMessage) {
		switch key {
		case "functionCall", "functionResponse", "inlineData", "inline_data", "fileData", "file_data", "executableCode", "codeExecutionResult", "thoughtSignature":
			excluded = true
		case "thought":
			var thought bool
			if json.Unmarshal(value, &thought) == nil && thought {
				excluded = true
			}
		}
	})
	return excluded
}

func oracleUniqueStringField(object json.RawMessage, field string) (string, bool) {
	count := 0
	value := ""
	valid := false
	if !oracleForEachObject(object, func(key string, raw json.RawMessage) {
		if key != field {
			return
		}
		count++
		if count == 1 && json.Unmarshal(raw, &value) == nil {
			valid = true
		}
	}) {
		return "", false
	}
	return value, count == 1 && valid
}

func appendOracleToken(tokens *[][]byte, raw json.RawMessage) {
	trimmed := bytes.TrimSpace(raw)
	*tokens = append(*tokens, append([]byte(nil), trimmed...))
}

func oracleForEachObject(raw []byte, visit func(string, json.RawMessage)) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return false
		}
		key, ok := keyToken.(string)
		if !ok {
			return false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return false
		}
		visit(key, value)
	}
	token, err = decoder.Token()
	return err == nil && token == json.Delim('}')
}

func oracleForEachArray(raw []byte, visit func(json.RawMessage)) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return false
	}
	for decoder.More() {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return false
		}
		visit(value)
	}
	token, err = decoder.Token()
	return err == nil && token == json.Delim(']')
}
