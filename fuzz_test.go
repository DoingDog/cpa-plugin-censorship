package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const maxFuzzJSONDepth = 256

type fuzzBlock struct {
	Term string
	Role string
}

type fuzzRuleResult struct {
	Texts       []string
	SpanChanged []bool
	Changed     bool
	Blocked     *fuzzBlock
}

type oracleTextMatch struct {
	Start int
	End   int
}

type oracleProtocolSpan struct {
	Text string
	Role string
}

func TestValidJSONObjectRejectsNonJSONWhitespace(t *testing.T) {
	for _, body := range [][]byte{[]byte("{}\v"), []byte("{}\f"), []byte("\v{}")} {
		if validJSONObject(body) {
			t.Errorf("validJSONObject(%q) = true", body)
		}
	}
	if body := []byte(" \t\r\n{}\n"); !validJSONObject(body) {
		t.Errorf("validJSONObject(%q) = false", body)
	}
}

func TestProtocolOracleRejectsWrongResults(t *testing.T) {
	matching := []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`)
	excluded := []byte(`{"tools":[{"description":"SECRET"}]}`)
	cases := []struct {
		name string
		body []byte
		mode mode
		got  transformResult
	}{
		{name: "missing transform", body: matching, mode: modeStrip},
		{name: "spurious block", body: excluded, mode: modeBlock, got: transformResult{Blocked: &blockMatch{Term: "SECRET", Role: "user"}}},
		{name: "unchanged transform", body: matching, mode: modeStrip, got: transformResult{Body: matching}},
		{name: "duplicate member accepted", body: []byte(`{"messages":[],"messages":[]}`), mode: modeStrip},
	}
	for _, tc := range cases {
		if err := checkProtocolResult("openai", tc.body, tc.mode, false, tc.got); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
}

func FuzzRuleEngineAgainstOracle(f *testing.F) {
	for _, seed := range []struct {
		text, terms string
		mode, fold  uint8
	}{
		{"ababa", "aba|ba", 0, 0},
		{"ALPHA Alpha", "Alpha", 1, 1},
		{"ςΣσ", "Σ", 1, 1},
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

func FuzzDuplicateWalkerAgainstOracle(f *testing.F) {
	for _, body := range [][]byte{
		[]byte(`{"x":0,"\u0078":1}`),
		[]byte(`{"\ud800\u0061":0,"\ufffda":1}`),
		[]byte(`{"\ud800\u0061":0,"\ufffd":1}`),
		[]byte(`{"left":{"x":0},"right":{"x":1}}`),
		[]byte(`{"items":[{"x":0},{"nested":{"x":1,"x":2}}]}`),
		[]byte(`{"value":"{\"x\":1,\"x\":2}"}`),
		[]byte(`{"tools":[{"x":0,"x":1}],"media":{"x":0,"x":1}}`),
	} {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 64<<10 || !boundedJSONNesting(body, maxFuzzJSONDepth) || !gjson.ValidBytes(body) {
			t.Skip()
		}
		got := hasDuplicateJSONMembers(gjson.ParseBytes(body))
		want := oracleHasDuplicateJSONMembers(body)
		if got != want {
			t.Fatalf("hasDuplicateJSONMembers(%s) = %t, oracle = %t", body, got, want)
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
		{format: "gemini", body: []byte(`{"contents":[{"role":"user","parts":[{"text":"SECRET","thought":false,"thought":true}]}]}`)},
		{format: "interactions", body: interactionStepsSeed(64)},
		{format: "interactions", body: []byte(`{"input":{"type":"user_input","content":[{"type":"text","text":"SECRET","inlineData":{"data":"SECRET"}}]}}`)},
		{format: "interactions", body: []byte(`{"input":[{"type":"function_result","content":"SECRET"}]}`)},
		{format: "openai", body: []byte(`not-json`)},
	}
	for _, seed := range seeds {
		for mode := uint8(0); mode < 3; mode++ {
			f.Add(seed.format, seed.body, mode, false)
			f.Add(seed.format, seed.body, mode, true)
		}
	}
	f.Fuzz(func(t *testing.T, format string, body []byte, modeByte uint8, fold bool) {
		if len(format) > 32 || len(body) > 64<<10 || !boundedJSONNesting(body, maxFuzzJSONDepth) {
			t.Skip()
		}
		modes := []string{"block", "strip", "obfs"}
		cfg := mustConfig(t, fmt.Sprintf("mode: %s\nignore_case: %t\nwords: [SECRET]\n", modes[modeByte%3], fold))
		got, err := transformRequest(body, format, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkProtocolResult(format, body, cfg.Mode, fold, got); err != nil {
			t.Fatal(err)
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

func checkProtocolResult(format string, body []byte, selected mode, fold bool, got transformResult) error {
	if !knownFormat(format) {
		if got.Invalid || got.Blocked != nil || len(got.Body) != 0 {
			return fmt.Errorf("unknown format returned %#v", got)
		}
		return nil
	}
	if !validJSONObject(body) {
		if !got.Invalid || got.Blocked != nil || len(got.Body) != 0 {
			return fmt.Errorf("invalid request returned %#v", got)
		}
		return nil
	}
	if oracleHasDuplicateJSONMembers(body) {
		if !got.Invalid || got.Blocked != nil || len(got.Body) != 0 {
			return fmt.Errorf("duplicate JSON object members returned %#v", got)
		}
		return nil
	}
	if got.Invalid {
		return fmt.Errorf("valid JSON object marked invalid: %s", body)
	}

	before, ok := oracleProtocolSpans(format, body)
	if !ok {
		return fmt.Errorf("oracle could not parse valid %s object", format)
	}
	matchingRoles := make(map[string]struct{})
	for _, span := range before {
		if oracleContains(span.Text, "SECRET", fold) {
			matchingRoles[span.Role] = struct{}{}
		}
	}
	matched := len(matchingRoles) != 0

	switch selected {
	case modeBlock:
		if len(got.Body) != 0 {
			return fmt.Errorf("block returned replacement body")
		}
		if !matched {
			if got.Blocked != nil {
				return fmt.Errorf("request without eligible match was blocked: %#v", got.Blocked)
			}
			return nil
		}
		if got.Blocked == nil || got.Blocked.Term != "SECRET" {
			return fmt.Errorf("eligible match was not blocked with YAML term: %#v", got.Blocked)
		}
		if _, ok := matchingRoles[got.Blocked.Role]; !ok {
			return fmt.Errorf("blocked role %q has no eligible match", got.Blocked.Role)
		}
		return nil
	case modeStrip, modeObfs:
		if got.Blocked != nil {
			return fmt.Errorf("transform mode blocked request: %#v", got.Blocked)
		}
		if !matched {
			if len(got.Body) != 0 {
				return fmt.Errorf("request without eligible match was rebuilt")
			}
			return nil
		}
		if len(got.Body) == 0 {
			return fmt.Errorf("eligible match returned no replacement body")
		}
	default:
		return fmt.Errorf("unknown mode %q", selected)
	}

	if !json.Valid(got.Body) {
		return fmt.Errorf("changed output is invalid JSON: %s", got.Body)
	}
	after, ok := oracleProtocolSpans(format, got.Body)
	if !ok || len(after) != len(before) {
		return fmt.Errorf("eligible spans changed shape: before=%#v after=%#v", before, after)
	}
	for i := range before {
		want := oracleStrip(before[i].Text, "SECRET", fold)
		if selected == modeObfs {
			want = oracleObfuscate(before[i].Text, "SECRET", fold, "​")
		}
		if after[i].Role != before[i].Role || after[i].Text != want {
			return fmt.Errorf("eligible span %d = %#v, want role %q text %q", i, after[i], before[i].Role, want)
		}
	}
	beforeExcluded := oracleExcludedTokens(format, body)
	if afterExcluded := oracleExcludedTokens(format, got.Body); !reflect.DeepEqual(afterExcluded, beforeExcluded) {
		return fmt.Errorf("excluded raw tokens changed: before=%q after=%q", beforeExcluded, afterExcluded)
	}
	return nil
}

func oracleProtocolSpans(format string, body []byte) ([]oracleProtocolSpan, bool) {
	if !validJSONObject(body) {
		return nil, false
	}
	var spans []oracleProtocolSpan
	switch format {
	case "openai":
		oracleOpenAISpans(body, &spans)
	case "openai-response":
		oracleOpenAIResponseSpans(body, &spans)
	case "claude":
		oracleClaudeSpans(body, &spans)
	case "gemini":
		oracleGeminiSpans(body, &spans)
	case "interactions":
		oracleInteractionsSpans(body, &spans)
	default:
		return nil, false
	}
	return spans, true
}

func oracleRoleEnabled(role string) bool {
	return role == "system" || role == "developer" || role == "user"
}

func oracleAppendString(spans *[]oracleProtocolSpan, raw json.RawMessage, role string) {
	if !oracleRoleEnabled(role) {
		return
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		*spans = append(*spans, oracleProtocolSpan{Text: text, Role: role})
	}
}

func oracleFirstField(object []byte, field string) (json.RawMessage, bool) {
	var result json.RawMessage
	found := false
	ok := oracleForEachObject(object, func(key string, value json.RawMessage) {
		if !found && key == field {
			result = append(json.RawMessage(nil), value...)
			found = true
		}
	})
	return result, ok && found
}

func oracleStringField(object []byte, field string) (string, bool) {
	raw, ok := oracleFirstField(object, field)
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func oracleOpenAISpans(root []byte, spans *[]oracleProtocolSpan) {
	messages, ok := oracleFirstField(root, "messages")
	if !ok {
		return
	}
	oracleForEachArray(messages, func(message json.RawMessage) {
		role, ok := oracleStringField(message, "role")
		if !ok || !oracleRoleEnabled(role) {
			return
		}
		content, ok := oracleFirstField(message, "content")
		if !ok {
			return
		}
		oracleAppendString(spans, content, role)
		oracleForEachArray(content, func(part json.RawMessage) {
			if partType, ok := oracleStringField(part, "type"); ok && partType == "text" {
				if text, ok := oracleFirstField(part, "text"); ok {
					oracleAppendString(spans, text, role)
				}
			}
		})
	})
}

func oracleOpenAIResponseSpans(root []byte, spans *[]oracleProtocolSpan) {
	if instructions, ok := oracleFirstField(root, "instructions"); ok {
		oracleAppendString(spans, instructions, "system")
	}
	input, ok := oracleFirstField(root, "input")
	if !ok {
		return
	}
	oracleAppendString(spans, input, "user")
	oracleForEachArray(input, func(item json.RawMessage) {
		if itemType, exists := oracleFirstField(item, "type"); exists {
			var value string
			if json.Unmarshal(itemType, &value) != nil || value != "" && value != "message" {
				return
			}
		}
		role, ok := oracleStringField(item, "role")
		if !ok || !oracleRoleEnabled(role) {
			return
		}
		content, ok := oracleFirstField(item, "content")
		if !ok {
			return
		}
		oracleAppendString(spans, content, role)
		oracleForEachArray(content, func(part json.RawMessage) {
			partRole := role
			partType := ""
			if rawType, exists := oracleFirstField(part, "type"); exists {
				if json.Unmarshal(rawType, &partType) != nil {
					return
				}
			}
			switch partType {
			case "output_text", "refusal":
				partRole = "assistant"
			}
			switch partType {
			case "refusal":
				if refusal, ok := oracleFirstField(part, "refusal"); ok {
					oracleAppendString(spans, refusal, partRole)
				}
			case "", "input_text", "output_text":
				if text, ok := oracleFirstField(part, "text"); ok {
					oracleAppendString(spans, text, partRole)
				}
			}
		})
	})
}

func oracleClaudeSpans(root []byte, spans *[]oracleProtocolSpan) {
	if system, ok := oracleFirstField(root, "system"); ok {
		oracleAppendString(spans, system, "system")
		oracleForEachArray(system, func(block json.RawMessage) {
			if blockType, ok := oracleStringField(block, "type"); ok && blockType == "text" {
				if text, ok := oracleFirstField(block, "text"); ok {
					oracleAppendString(spans, text, "system")
				}
			}
		})
	}
	messages, ok := oracleFirstField(root, "messages")
	if !ok {
		return
	}
	oracleForEachArray(messages, func(message json.RawMessage) {
		role, ok := oracleStringField(message, "role")
		if !ok || role != "system" && role != "user" {
			return
		}
		content, ok := oracleFirstField(message, "content")
		if !ok {
			return
		}
		oracleAppendString(spans, content, role)
		oracleForEachArray(content, func(block json.RawMessage) {
			if blockType, ok := oracleStringField(block, "type"); ok && blockType == "text" {
				if text, ok := oracleFirstField(block, "text"); ok {
					oracleAppendString(spans, text, role)
				}
			}
		})
	})
}

func oracleGeminiSpans(root []byte, spans *[]oracleProtocolSpan) {
	for _, name := range []string{"systemInstruction", "system_instruction"} {
		if instruction, ok := oracleFirstField(root, name); ok {
			oracleGeminiParts(instruction, "system", spans)
		}
	}
	contents, ok := oracleFirstField(root, "contents")
	if !ok {
		return
	}
	previousRole := ""
	oracleForEachArray(contents, func(content json.RawMessage) {
		role := ""
		if rawRole, exists := oracleFirstField(content, "role"); exists {
			var value string
			if json.Unmarshal(rawRole, &value) != nil {
				previousRole = oracleNextGeminiRole(previousRole)
				return
			}
			switch value {
			case "user":
				role = "user"
				previousRole = value
			case "model":
				role = "assistant"
				previousRole = value
			default:
				previousRole = oracleNextGeminiRole(previousRole)
				return
			}
		} else {
			previousRole = oracleNextGeminiRole(previousRole)
			if previousRole == "user" {
				role = "user"
			} else {
				role = "assistant"
			}
		}
		oracleGeminiParts(content, role, spans)
	})
}

func oracleNextGeminiRole(previousRole string) string {
	if previousRole == "" || previousRole == "model" {
		return "user"
	}
	return "model"
}

func oracleGeminiParts(container json.RawMessage, role string, spans *[]oracleProtocolSpan) {
	parts, ok := oracleFirstField(container, "parts")
	if !ok {
		return
	}
	oracleForEachArray(parts, func(part json.RawMessage) {
		if oracleGeminiPartExcluded(part) {
			return
		}
		if text, ok := oracleFirstField(part, "text"); ok {
			oracleAppendString(spans, text, role)
		}
	})
}

func oracleInteractionsSpans(root []byte, spans *[]oracleProtocolSpan) {
	system, ok := oracleFirstField(root, "system_instruction")
	if !ok {
		system, ok = oracleFirstField(root, "systemInstruction")
	}
	if ok {
		oracleAppendString(spans, system, "system")
		if text, ok := oracleFirstField(system, "text"); ok {
			oracleAppendString(spans, text, "system")
		}
		oracleInteractionParts(system, "system", spans)
	}
	input, ok := oracleFirstField(root, "input")
	if !ok {
		return
	}
	oracleAppendString(spans, input, "user")
	if bytes.HasPrefix(bytes.TrimSpace(input), []byte("{")) {
		oracleInteractionItem(input, "user", spans, 0)
		return
	}
	oracleForEachArray(input, func(item json.RawMessage) {
		oracleAppendString(spans, item, "user")
		if bytes.HasPrefix(bytes.TrimSpace(item), []byte("{")) {
			oracleInteractionItem(item, "user", spans, 0)
		}
	})
}

func oracleInteractionItem(item json.RawMessage, inheritedRole string, spans *[]oracleProtocolSpan, depth int) {
	if depth > maxFuzzJSONDepth {
		return
	}
	role := inheritedRole
	if rawRole, exists := oracleFirstField(item, "role"); exists {
		var value string
		if json.Unmarshal(rawRole, &value) != nil {
			return
		}
		switch value {
		case "user":
			role = "user"
		case "model", "assistant":
			role = "assistant"
		default:
			return
		}
	}
	if rawType, exists := oracleFirstField(item, "type"); exists {
		var value string
		if json.Unmarshal(rawType, &value) != nil {
			return
		}
		switch value {
		case "", "user_input":
		case "model_output":
			role = "assistant"
		default:
			return
		}
	}
	if content, ok := oracleFirstField(item, "content"); ok {
		oracleAppendString(spans, content, role)
		if bytes.HasPrefix(bytes.TrimSpace(content), []byte("{")) {
			oracleInteractionPart(content, role, spans)
		} else {
			oracleForEachArray(content, func(part json.RawMessage) {
				oracleInteractionPart(part, role, spans)
			})
		}
	}
	oracleInteractionParts(item, role, spans)
	if steps, ok := oracleFirstField(item, "steps"); ok {
		oracleForEachArray(steps, func(step json.RawMessage) {
			if bytes.HasPrefix(bytes.TrimSpace(step), []byte("{")) {
				oracleInteractionItem(step, role, spans, depth+1)
			}
		})
	}
}

func oracleInteractionParts(container json.RawMessage, role string, spans *[]oracleProtocolSpan) {
	parts, ok := oracleFirstField(container, "parts")
	if !ok {
		return
	}
	oracleForEachArray(parts, func(part json.RawMessage) {
		oracleInteractionPart(part, role, spans)
	})
}

func oracleInteractionPart(part json.RawMessage, role string, spans *[]oracleProtocolSpan) {
	if !oracleInteractionPartAllowed(part) {
		return
	}
	if text, ok := oracleFirstField(part, "text"); ok {
		oracleAppendString(spans, text, role)
	}
}

func oracleInteractionPartAllowed(part json.RawMessage) bool {
	if !bytes.HasPrefix(bytes.TrimSpace(part), []byte("{")) {
		return false
	}
	if partType, exists := oracleFirstField(part, "type"); exists {
		var value string
		if json.Unmarshal(partType, &value) != nil || value != "" && value != "text" {
			return false
		}
	}
	return !oracleGeminiPartExcluded(part)
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
		Mode:         selected,
		IgnoreCase:   fold,
		Rules:        rules,
		ObfsChar:     "​",
		BlockMatcher: newFoldMatcher(rules),
	}
}

func TestSnapshotForFuzzPrecompilesFoldBlockMatcher(t *testing.T) {
	cfg := snapshotForFuzz([]string{"SECRET"}, 0, true)
	if cfg == nil {
		t.Fatal("snapshotForFuzz returned nil")
	}
	if cfg.BlockMatcher == nil {
		t.Fatal("snapshotForFuzz left BlockMatcher nil")
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
	blocked, changed := applyMode(spans, cfg)
	result := fuzzRuleResult{
		Texts:       make([]string, len(spans)),
		SpanChanged: make([]bool, len(spans)),
		Changed:     changed,
	}
	for i := range spans {
		result.Texts[i] = spans[i].Text
		result.SpanChanged[i] = spans[i].Changed
	}
	if blocked != nil {
		result.Blocked = &fuzzBlock{Term: blocked.Term, Role: blocked.Role}
	}
	return result
}

func oracleApply(texts []string, cfg *configSnapshot) fuzzRuleResult {
	result := fuzzRuleResult{
		Texts:       append([]string(nil), texts...),
		SpanChanged: make([]bool, len(texts)),
	}
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
				next := oracleStrip(text, rule.Term, cfg.IgnoreCase)
				if next != text {
					result.Texts[i] = next
					result.SpanChanged[i] = true
					result.Changed = true
				}
			}
		}
	case modeObfs:
		for _, rule := range cfg.Rules {
			for i, text := range result.Texts {
				next := oracleObfuscate(text, rule.Term, cfg.IgnoreCase, cfg.ObfsChar)
				if next != text {
					result.Texts[i] = next
					result.SpanChanged[i] = true
					result.Changed = true
				}
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
	if !json.Valid(body) {
		return false
	}
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
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
		case "interactions":
			switch key {
			case "system_instruction", "systemInstruction":
				oracleInteractionExcludedParts(value, &tokens)
			case "input":
				oracleInteractionsExcluded(value, "user", &tokens, 0)
			}
		}
	})
	return tokens
}

func oracleInteractionExcludedParts(container json.RawMessage, tokens *[][]byte) {
	parts, ok := oracleFirstField(container, "parts")
	if !ok {
		return
	}
	oracleForEachArray(parts, func(part json.RawMessage) {
		if !oracleInteractionPartAllowed(part) {
			appendOracleToken(tokens, part)
		}
	})
}

func oracleInteractionsExcluded(raw json.RawMessage, inheritedRole string, tokens *[][]byte, depth int) {
	if depth > maxFuzzJSONDepth {
		return
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.HasPrefix(trimmed, []byte("[")) {
		oracleForEachArray(raw, func(item json.RawMessage) {
			oracleInteractionsExcluded(item, inheritedRole, tokens, depth)
		})
		return
	}
	if !bytes.HasPrefix(trimmed, []byte("{")) {
		return
	}

	role := inheritedRole
	if rawRole, exists := oracleFirstField(raw, "role"); exists {
		var value string
		if json.Unmarshal(rawRole, &value) != nil {
			appendOracleToken(tokens, raw)
			return
		}
		switch value {
		case "user":
			role = "user"
		case "model", "assistant":
			role = "assistant"
		default:
			appendOracleToken(tokens, raw)
			return
		}
	}
	if rawType, exists := oracleFirstField(raw, "type"); exists {
		var value string
		if json.Unmarshal(rawType, &value) != nil {
			appendOracleToken(tokens, raw)
			return
		}
		switch value {
		case "", "user_input":
		case "model_output":
			role = "assistant"
		default:
			appendOracleToken(tokens, raw)
			return
		}
	}

	if content, ok := oracleFirstField(raw, "content"); ok {
		contentTrimmed := bytes.TrimSpace(content)
		if bytes.HasPrefix(contentTrimmed, []byte("{")) {
			if !oracleInteractionPartAllowed(content) {
				appendOracleToken(tokens, content)
			}
		} else {
			oracleForEachArray(content, func(part json.RawMessage) {
				if !oracleInteractionPartAllowed(part) {
					appendOracleToken(tokens, part)
				}
			})
		}
	}
	oracleInteractionExcludedParts(raw, tokens)
	if steps, ok := oracleFirstField(raw, "steps"); ok {
		oracleForEachArray(steps, func(step json.RawMessage) {
			oracleInteractionsExcluded(step, role, tokens, depth+1)
		})
	}
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

func TestOracleGeminiPartExcludesNestedMachineFields(t *testing.T) {
	for _, part := range []json.RawMessage{
		[]byte(`{"text":"SECRET","functionCall":{"thought_signature":"sig"}}`),
		[]byte(`{"text":"SECRET","functionResponse":{"thought_signature":"sig"}}`),
		[]byte(`{"text":"SECRET","extra_content":{"google":{"thought_signature":"sig"}}}`),
	} {
		if !oracleGeminiPartExcluded(part) {
			t.Errorf("oracleGeminiPartExcluded(%s) = false", part)
		}
	}
}

func oracleGeminiPartExcluded(part json.RawMessage) bool {
	excluded := false
	seen := make(map[string]struct{})
	oracleForEachObject(part, func(key string, value json.RawMessage) {
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		switch key {
		case "functionCall", "functionResponse", "function_call", "function_response", "inlineData", "inline_data", "fileData", "file_data", "executableCode", "executable_code", "codeExecutionResult", "code_execution_result", "thoughtSignature", "thought_signature":
			excluded = true
		case "thought":
			var thought bool
			if json.Unmarshal(value, &thought) == nil && thought {
				excluded = true
			}
		}
	})
	for _, path := range [][]string{
		{"functionCall", "thoughtSignature"},
		{"functionCall", "thought_signature"},
		{"functionResponse", "thoughtSignature"},
		{"functionResponse", "thought_signature"},
		{"extra_content", "google", "thought_signature"},
	} {
		if oracleJSONFieldPathExists(part, path...) {
			return true
		}
	}
	return excluded
}

func oracleJSONFieldPathExists(raw json.RawMessage, path ...string) bool {
	if len(path) == 0 {
		return true
	}
	value, ok := oracleFirstField(raw, path[0])
	if !ok {
		return false
	}
	return len(path) == 1 || oracleJSONFieldPathExists(value, path[1:]...)
}

func oracleHasDuplicateJSONMembers(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		seen := make(map[string]struct{})
		duplicate := false
		if !oracleForEachObject(raw, func(key string, value json.RawMessage) {
			if _, exists := seen[key]; exists {
				duplicate = true
				return
			}
			seen[key] = struct{}{}
			if !duplicate && oracleHasDuplicateJSONMembers(value) {
				duplicate = true
			}
		}) {
			return false
		}
		return duplicate
	}
	if bytes.HasPrefix(trimmed, []byte("[")) {
		duplicate := false
		if !oracleForEachArray(raw, func(value json.RawMessage) {
			if !duplicate && oracleHasDuplicateJSONMembers(value) {
				duplicate = true
			}
		}) {
			return false
		}
		return duplicate
	}
	return false
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

func FuzzRebuildBodyAgainstMarshalOracle(f *testing.F) {
	for _, seed := range []struct {
		first, second string
		mask          uint8
	}{
		{"first", "second", 3},
		{"<>&\u2028\u2029", "newline\ntext", 1},
		{"", "unchanged", 0},
	} {
		f.Add(seed.first, seed.second, seed.mask)
	}
	f.Fuzz(func(t *testing.T, first, second string, mask uint8) {
		if len(first) > 1024 || len(second) > 1024 {
			t.Skip()
		}

		firstToken, err := json.Marshal("original:" + first)
		if err != nil {
			t.Fatal(err)
		}
		secondToken, err := json.Marshal("original:" + second)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 0, len(firstToken)+len(secondToken)+96)
		body = append(body, " \n { \"first\" : "...)
		firstStart := len(body)
		body = append(body, firstToken...)
		firstEnd := len(body)
		body = append(body, " , \"number\" : 1e+03 , \"unknown\" : \"\u003c\" , \"second\" : "...)
		secondStart := len(body)
		body = append(body, secondToken...)
		secondEnd := len(body)
		body = append(body, " , \"media\" : \"AA==\" } \n "...)
		spans := []textSpan{
			{RawStart: firstStart, RawEnd: firstEnd, Text: first, Changed: mask&1 != 0},
			{RawStart: secondStart, RawEnd: secondEnd, Text: second, Changed: mask&2 != 0},
		}

		got, gotErr := rebuildBody(body, spans)
		want, wantErr := marshalRebuildOracle(body, spans)
		if (gotErr != nil) != (wantErr != nil) {
			t.Fatalf("error = %v, oracle error = %v", gotErr, wantErr)
		}
		if gotErr != nil {
			return
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("body = %q, want %q", got, want)
		}
	})
}

func marshalRebuildOracle(body []byte, spans []textSpan) ([]byte, error) {
	type replacement struct {
		start int
		end   int
		bytes []byte
	}

	previousEnd := 0
	finalLen := len(body)
	replacements := make([]replacement, 0, len(spans))
	for _, span := range spans {
		if !span.Changed {
			continue
		}
		if span.RawStart < 0 || span.RawStart >= span.RawEnd || span.RawEnd > len(body) || span.RawStart < previousEnd {
			return nil, fmt.Errorf("invalid changed span")
		}
		encoded, err := json.Marshal(span.Text)
		if err != nil {
			return nil, err
		}
		finalLen += len(encoded) - (span.RawEnd - span.RawStart)
		replacements = append(replacements, replacement{start: span.RawStart, end: span.RawEnd, bytes: encoded})
		previousEnd = span.RawEnd
	}
	if len(replacements) == 0 {
		return nil, nil
	}

	out := make([]byte, finalLen)
	source, destination := len(body), len(out)
	for i := len(replacements) - 1; i >= 0; i-- {
		replacement := replacements[i]
		tailLen := source - replacement.end
		destination -= tailLen
		copy(out[destination:destination+tailLen], body[replacement.end:source])
		destination -= len(replacement.bytes)
		copy(out[destination:destination+len(replacement.bytes)], replacement.bytes)
		source = replacement.start
	}
	destination -= source
	copy(out[destination:destination+source], body[:source])
	return out, nil
}
