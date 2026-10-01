package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/tidwall/gjson"
)

func TestOpenAIBlockUsesRuleOrderBeforeDocumentOrder(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [ab, a]\n")
	body := []byte(`{"messages":[{"role":"user","content":"a"},{"role":"developer","content":"ab"}]}`)
	resp := interceptRPC(t, "openai", body)
	if !resp.Terminate || resp.StatusCode != 400 || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
	}
	if string(resp.ResponseBody) != `{"error":{"type":"invalid_request_error","code":"censorship_blocked","message":"request blocked by censorship rule","term":"ab","role":"developer"}}` {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}
}

func TestOpenAIBlockDocumentOrderAndErrorEscaping(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [hit]\n")
	resp := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"system","content":"hit"},{"role":"user","content":"hit"}]}`))
	if gjson.GetBytes(resp.ResponseBody, "error.role").String() != "system" {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}

	term := "quote \" and\nnewline\n"
	registerConfig(t, "mode: block\nwords:\n  - |\n    quote \" and\n    newline\n")
	body, err := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": term}}})
	if err != nil {
		t.Fatal(err)
	}
	resp = interceptRPC(t, "openai", body)
	if !json.Valid(resp.ResponseBody) || gjson.GetBytes(resp.ResponseBody, "error.term").String() != term {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}
}

func assertBlockedRole(t *testing.T, sourceFormat, body, wantRole string) {
	t.Helper()
	registerConfig(t, "mode: block\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	resp := interceptRPC(t, sourceFormat, []byte(body))
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "SECRET" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != wantRole {
		t.Fatalf("format=%s role=%s response=%#v body=%s", sourceFormat, wantRole, resp, resp.ResponseBody)
	}
}

func TestSignedRewriteCancellationLeavesRequestUntouched(t *testing.T) {
	registerConfig(t, "words:\n  strip: [\"​\"]\n  obfs: [ab]\nscope:\n  roles: [assistant]\n")
	resp := interceptRPC(t, "gemini", []byte(`{"contents":[{"role":"model","parts":[{"text":"a​b","thoughtSignature":"c2ln"}]}]}`))
	if resp.Terminate || len(resp.Body) != 0 || len(resp.ResponseBody) != 0 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestMixedRulesBlockOriginalTextBeforeRewrites(t *testing.T) {
	cfg := &configSnapshot{
		Mode: modeStrip,
		Rules: []compiledRule{
			{Term: "BLOCK"},
			{Term: "strip"},
			{Term: "obfs", ExactReplacement: "o* bfs"},
		},
		BlockEnd:  1,
		StripEnd:  2,
		rangesSet: true,
	}
	spans := []textSpan{{Text: "BLOCK strip obfs", Role: "user"}}

	blocked, changed := applyMode(spans, cfg)
	if changed || blocked == nil || blocked.Term != "BLOCK" || blocked.Role != "user" || spans[0].Text != "BLOCK strip obfs" {
		t.Fatalf("applyMode() = %#v, %t, %#v; want original-text block", blocked, changed, spans[0])
	}
}

func TestMixedRulesRewriteRangesKeepPhaseAndRuleOrder(t *testing.T) {
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: true,
		Rules: []compiledRule{
			{Term: "BLOCK", Runes: []rune("BLOCK")},
			{Term: "x", Runes: []rune("x")},
			{Term: "ab", Runes: []rune("ab")},
			{Term: "c", Runes: []rune("c")},
			{Term: "c", Runes: []rune("c")},
		},
		BlockEnd:       1,
		StripEnd:       3,
		rangesSet:      true,
		ObfsChar:       "*",
		BlockMatcher:   newFoldMatcher([]compiledRule{{Term: "BLOCK", Runes: []rune("BLOCK")}}),
		RewriteMatcher: newFoldMatcher([]compiledRule{{Term: "x", Runes: []rune("x")}, {Term: "ab", Runes: []rune("ab")}, {Term: "c", Runes: []rune("c")}, {Term: "c", Runes: []rune("c")}}),
	}
	spans := []textSpan{{Text: "aXbC", Role: "user"}}

	blocked, changed := applyMode(spans, cfg)
	if blocked != nil || !changed || spans[0].Text != "C**" {
		t.Fatalf("applyMode() = %#v, %t, %#v; want strip cascade then duplicate obfuscation", blocked, changed, spans[0])
	}
}

func TestMixedRulesFoldedRangesMatchSigmaAndKelvin(t *testing.T) {
	cfg := &configSnapshot{
		Mode:       modeObfs,
		IgnoreCase: true,
		Rules: []compiledRule{
			{Term: "Σ", Runes: []rune("Σ")},
			{Term: "K", Runes: []rune("K")},
		},
		BlockEnd:       1,
		StripEnd:       2,
		rangesSet:      true,
		BlockMatcher:   newFoldMatcher([]compiledRule{{Term: "Σ", Runes: []rune("Σ")}}),
		RewriteMatcher: newFoldMatcher([]compiledRule{{Term: "K", Runes: []rune("K")}}),
	}

	blocked, changed := applyMode([]textSpan{{Text: "ςK", Role: "developer"}}, cfg)
	if changed || blocked == nil || blocked.Term != "Σ" || blocked.Role != "developer" {
		t.Fatalf("applyMode() = %#v, %t; want Sigma block", blocked, changed)
	}

	spans := []textSpan{{Text: "K", Role: "user"}}
	blocked, changed = applyMode(spans, cfg)
	if blocked != nil || !changed || spans[0].Text != "" {
		t.Fatalf("applyMode() = %#v, %t, %#v; want Kelvin strip", blocked, changed, spans[0])
	}
}

func TestStripRemovesAllOccurrencesAcrossAllNodes(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [bad]\n")
	body := []byte(`{"messages":[{"role":"user","content":"bad bad"},{"role":"user","content":"xbadx"},{"role":"user","content":"bad/bad"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":" "},{"role":"user","content":"xx"},{"role":"user","content":"/"}]}`
	if string(got.Body) != want {
		t.Fatalf("body = %s, want %s", got.Body, want)
	}
}

func TestStripOrderedCascadeAndFoldedOccurrences(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nignore_case: true\nwords: [AB, x]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aBxABx"}]}`)
	got, _ := transformRequest(body, "openai", cfg)
	if string(got.Body) != `{"messages":[{"role":"user","content":""}]}` {
		t.Fatalf("body = %s", got.Body)
	}
}

func TestStripUsesLeftmostNonOverlappingOccurrences(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [aa]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aaa"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil || string(got.Body) != `{"messages":[{"role":"user","content":"a"}]}` {
		t.Fatalf("body = %s, err = %v", got.Body, err)
	}
}

func TestObfsPreservesMatchedCaseAndInsertsOncePerOccurrence(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [Alpha, 世界]\nobfs:\n  char: '⁠'\n")
	body := []byte(`{"messages":[{"role":"user","content":"ALPHA Alpha 世界世界"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":"A⁠LPHA A⁠lpha 世⁠界世⁠界"}]}`
	if string(got.Body) != want {
		t.Fatalf("body = %s, want %s", got.Body, want)
	}
}

func TestRebuildMatchesDecodedEscapesAndUsesEncodingJSONEscaping(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nwords: ['<X>']\n")
	u := string([]byte{0x5c, 'u'})
	encoded := u + "003cX" + u + "003e" + u + "0026" + u + "2028" + u + "2029"
	body := []byte(`{"messages":[{"role":"user","content":"` + encoded + `"}],"raw":"` + encoded + ` excluded"}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantToken := u + "003c" + "​" + "X" + u + "003e" + u + "0026" + u + "2028" + u + "2029"
	want := []byte(`{"messages":[{"role":"user","content":"` + wantToken + `"}],"raw":"` + encoded + ` excluded"}`)
	if !bytes.Equal(got.Body, want) {
		t.Fatalf("body = %q, want %q", got.Body, want)
	}
}

func TestTransformIsByteDeterministic(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [Alpha]\n")
	body := []byte(" { \"messages\" : [ { \"role\" : \"user\", \"content\" : \"ALPHA Alpha\" } ], \"n\":1e+03 } ")
	want := []byte(" { \"messages\" : [ { \"role\" : \"user\", \"content\" : \"A" + "​" + "LPHA A" + "​" + "lpha\" } ], \"n\":1e+03 } ")
	var first []byte
	var firstHash [32]byte
	for i := 0; i < 100; i++ {
		got, err := transformRequest(body, "openai", cfg)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(got.Body)
		if i == 0 {
			first = append([]byte(nil), got.Body...)
			firstHash = hash
			if !bytes.Equal(first, want) {
				t.Fatalf("span-external bytes changed: got %q, want %q", first, want)
			}
			continue
		}
		if !bytes.Equal(got.Body, first) || hash != firstHash {
			t.Fatalf("iteration %d was nondeterministic", i)
		}
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				got, err := transformRequest(body, "openai", cfg)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(got.Body, first) || sha256.Sum256(got.Body) != firstHash {
					errs <- fmt.Errorf("concurrent transform was nondeterministic")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestRebuildBodyValidatesBeforeWriting(t *testing.T) {
	body := []byte(`{"first":"one","second":"two"}`)
	first := textSpan{RawStart: 9, RawEnd: 14, Text: "changed", Changed: true}
	second := textSpan{RawStart: 24, RawEnd: 29, Text: "changed", Changed: true}
	cases := []struct {
		name  string
		spans []textSpan
	}{
		{"negative start", []textSpan{{RawStart: -1, RawEnd: 1, Changed: true}}},
		{"empty range", []textSpan{{RawStart: 9, RawEnd: 9, Changed: true}}},
		{"range past body", []textSpan{{RawStart: 24, RawEnd: len(body) + 1, Changed: true}}},
		{"out of order", []textSpan{second, first}},
		{"overlap", []textSpan{first, {RawStart: 13, RawEnd: 29, Changed: true}}},
		{"invalid span after valid span", []textSpan{first, {RawStart: -1, RawEnd: 1, Changed: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := rebuildBody(body, tc.spans)
			if !errors.Is(err, errInvalidSpan) {
				t.Fatalf("error = %v, want errInvalidSpan", err)
			}
			if out != nil {
				t.Fatalf("output = %q, want nil", out)
			}
		})
	}

	out, err := rebuildBody(body, []textSpan{{RawStart: -1, RawEnd: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("output without changed spans = %q, want nil", out)
	}
}

func TestRebuildBodyUsesEncoderDefaultEscaping(t *testing.T) {
	body := []byte(` { "first" : "old" , "number" : 1e+03 , "second" : "other" , "media" : "AA==" } `)
	firstStart := bytes.Index(body, []byte(`"old"`))
	firstEnd := firstStart + len(`"old"`)
	secondStart := bytes.Index(body, []byte(`"other"`))
	secondEnd := secondStart + len(`"other"`)
	firstText := "<>&\u2028\u2029" + string([]byte{0xff}) + "\n"
	secondText := "\n<&>\u2028\u2029" + string([]byte{0xfe})
	firstToken, err := json.Marshal(firstText)
	if err != nil {
		t.Fatal(err)
	}
	secondToken, err := json.Marshal(secondText)
	if err != nil {
		t.Fatal(err)
	}

	got, err := rebuildBody(body, []textSpan{
		{RawStart: firstStart, RawEnd: firstEnd, Text: firstText, Changed: true},
		{RawStart: secondStart, RawEnd: secondEnd, Text: secondText, Changed: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 0, len(body)+len(firstToken)+len(secondToken)-(firstEnd-firstStart)-(secondEnd-secondStart))
	want = append(want, body[:firstStart]...)
	want = append(want, firstToken...)
	want = append(want, body[firstEnd:secondStart]...)
	want = append(want, secondToken...)
	want = append(want, body[secondEnd:]...)
	if !bytes.Equal(got, want) {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if bytes.Contains(got, []byte{'\n'}) {
		t.Fatalf("body contains an encoder newline: %q", got)
	}
}

var rebuildBodyAllocationSink []byte
var applyModeAllocationSink bool

func TestApplyModeNonmatchingSinglePhaseDoesNotAllocate(t *testing.T) {
	if raceDetectorEnabled {
		t.Skip("allocation ceilings are measured without race instrumentation")
	}
	const spanCount = 10_000
	tests := []struct {
		name, yaml string
	}{
		{name: "block", yaml: "mode: block\nwords: [blocked]\n"},
		{name: "strip", yaml: "mode: strip\nwords: [stripped]\n"},
		{name: "obfs", yaml: "mode: obfs\nwords: [obfuscated]\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := mustConfig(t, test.yaml)
			spans := make([]textSpan, spanCount)
			for i := range spans {
				spans[i].Text = "safe"
			}

			allocations := testing.AllocsPerRun(100, func() {
				blocked, changed := applyMode(spans, cfg)
				applyModeAllocationSink = blocked != nil || changed
			})
			if applyModeAllocationSink {
				t.Fatal("applyMode() reported a match for safe spans")
			}
			if allocations != 0 {
				t.Fatalf("allocations = %.1f, want 0", allocations)
			}
		})
	}
}

func TestRebuildBodyAllocationCeiling(t *testing.T) {
	if raceDetectorEnabled {
		t.Skip("allocation ceilings are measured without race instrumentation")
	}
	const spanCount = 64
	const allocationCeiling = 8
	body := make([]byte, 0, spanCount*16)
	body = append(body, '[')
	spans := make([]textSpan, 0, spanCount)
	for i := 0; i < spanCount; i++ {
		if i > 0 {
			body = append(body, ',')
		}
		token, err := json.Marshal(fmt.Sprintf("original-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		start := len(body)
		body = append(body, token...)
		spans = append(spans, textSpan{
			RawStart: start,
			RawEnd:   len(body),
			Text:     fmt.Sprintf("replacement-%d", i),
			Changed:  true,
		})
	}
	body = append(body, ']')

	allocations := testing.AllocsPerRun(100, func() {
		out, err := rebuildBody(body, spans)
		if err != nil {
			t.Fatal(err)
		}
		rebuildBodyAllocationSink = out
	})
	if allocations > allocationCeiling {
		t.Fatalf("allocations = %.1f, ceiling = %d", allocations, allocationCeiling)
	}
}

func TestUseFoldRewritePreflightBoundary(t *testing.T) {
	tests := []struct {
		rules, textBytes int
		want             bool
	}{
		{rules: 7, textBytes: 4096},
		{rules: 8, textBytes: 4095},
		{rules: 8, textBytes: 4096, want: true},
		{rules: 32, textBytes: 16384, want: true},
	}
	for _, test := range tests {
		if got := useFoldRewritePreflight(test.rules, test.textBytes); got != test.want {
			t.Errorf("useFoldRewritePreflight(%d, %d) = %t, want %t", test.rules, test.textBytes, got, test.want)
		}
	}
}

func TestFoldRewritePreflightLeavesFlagsOutsideFoldedRewritePath(t *testing.T) {
	exactCfg := mustConfig(t, `mode: strip
words: [HIT]
`)
	noPreflightCfg := mustConfig(t, `mode: strip
ignore_case: true
words: [HIT]
`)
	noPreflightCfg.RewriteMatcher = nil

	for name, cfg := range map[string]*configSnapshot{
		"exact":        exactCfg,
		"no preflight": noPreflightCfg,
	} {
		t.Run(name, func(t *testing.T) {
			spans := []textSpan{{Text: strings.Repeat("z", 4096), SkipFoldRewrite: true}}
			blocked, changed := applyMode(spans, cfg)
			if blocked != nil || changed || !spans[0].SkipFoldRewrite {
				t.Fatalf("applyMode() = %#v, %t, SkipFoldRewrite=%t; want untouched preflight flag", blocked, changed, spans[0].SkipFoldRewrite)
			}
		})
	}
}

func TestFoldRewritePreflightMarksOnlyTotalMisses(t *testing.T) {
	cfg := mustConfig(t, `mode: strip
ignore_case: true
words: [HIT, q0, q1, q2, q3, q4, q5, q6]
`)
	spans := []textSpan{
		{Text: strings.Repeat("z", 4096), Role: "user"},
		{Text: strings.Repeat("z", 4096) + "hit", Role: "assistant"},
	}
	blocked, changed := applyMode(spans, cfg)
	if blocked != nil || !changed {
		t.Fatalf("applyMode() = %#v, %t; want rewrite", blocked, changed)
	}
	if !spans[0].SkipFoldRewrite || spans[0].Changed {
		t.Fatalf("total-miss span = %#v", spans[0])
	}
	if spans[1].SkipFoldRewrite || !spans[1].Changed || spans[1].Text != strings.Repeat("z", 4096) {
		t.Fatalf("matching span = %#v", spans[1])
	}

	small := []textSpan{{Text: strings.Repeat("z", 4095)}}
	applyMode(small, cfg)
	if small[0].SkipFoldRewrite {
		t.Fatalf("small folded span was preflighted: %#v", small[0])
	}

	exactCfg := mustConfig(t, `mode: strip
words: [HIT, q0, q1, q2, q3, q4, q5, q6]
`)
	exact := []textSpan{{Text: strings.Repeat("z", 4096)}}
	applyMode(exact, exactCfg)
	if exact[0].SkipFoldRewrite {
		t.Fatalf("exact span was preflighted: %#v", exact[0])
	}
}

func TestFoldRewritePreflightPreservesRuleMajorCascade(t *testing.T) {
	cfg := mustConfig(t, `mode: strip
ignore_case: true
words: [X, ab, q0, q1, q2, q3, q4, q5]
`)
	padding := strings.Repeat("z", 4093)
	spans := []textSpan{{Text: "aXb" + padding, Role: "user"}}
	blocked, changed := applyMode(spans, cfg)
	if blocked != nil || !changed || spans[0].SkipFoldRewrite || spans[0].Text != padding {
		t.Fatalf("applyMode() = %#v, %t, span %#v; want rule-major cascade", blocked, changed, spans[0])
	}
}

func TestFoldRewritePreflightKeepsSpansIsolated(t *testing.T) {
	cfg := mustConfig(t, `mode: strip
ignore_case: true
words: [ab, q0, q1, q2, q3, q4, q5, q6]
`)
	spans := []textSpan{
		{Text: strings.Repeat("z", 4095) + "a", Role: "user"},
		{Text: "b" + strings.Repeat("z", 4095), Role: "assistant"},
	}
	blocked, changed := applyMode(spans, cfg)
	if blocked != nil || changed {
		t.Fatalf("applyMode() = %#v, %t; want no cross-span match", blocked, changed)
	}
	if !spans[0].SkipFoldRewrite || !spans[1].SkipFoldRewrite {
		t.Fatalf("isolated spans were not independently skipped: %#v", spans)
	}
}

func TestExactBlockAcrossSpans(t *testing.T) {
	const ruleCount = 256
	var words strings.Builder
	words.WriteString("mode: block\nwords:\n")
	for i := 0; i < ruleCount; i++ {
		fmt.Fprintf(&words, "  - rule-%03d\n", i)
	}
	cfg := mustConfig(t, words.String())

	tests := []struct {
		name  string
		spans []textSpan
	}{
		{
			name: "rule 0 wins by rule order",
			spans: []textSpan{
				{Text: "system sees rule-127 and rule-255", Role: "system"},
				{Text: "user sees rule-000 and rule-127", Role: "user"},
				{Text: "assistant sees rule-255", Role: "assistant"},
			},
		},
		{
			name: "rule 127 wins earliest document span",
			spans: []textSpan{
				{Text: "system sees rule-127", Role: "system"},
				{Text: "user sees rule-127 and rule-255", Role: "user"},
				{Text: "assistant sees rule-255", Role: "assistant"},
			},
		},
		{
			name: "rule 255 wins earliest document span",
			spans: []textSpan{
				{Text: "system sees no configured term", Role: "system"},
				{Text: "user sees rule-255", Role: "user"},
				{Text: "assistant sees rule-255", Role: "assistant"},
			},
		},
		{
			name: "no match",
			spans: []textSpan{
				{Text: "system sees no configured term", Role: "system"},
				{Text: "user sees no configured term", Role: "user"},
				{Text: "assistant sees no configured term", Role: "assistant"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantRule, wantRole, wantMatched := -1, "", false
			for ruleIndex, rule := range cfg.Rules {
				for _, span := range tc.spans {
					if strings.Contains(span.Text, rule.Term) {
						wantRule, wantRole, wantMatched = ruleIndex, span.Role, true
						break
					}
				}
				if wantMatched {
					break
				}
			}

			gotRule, gotRole, gotMatched := matchExactBlock(tc.spans, cfg)
			if gotRule != wantRule || gotRole != wantRole || gotMatched != wantMatched {
				t.Fatalf("matchExactBlock() = (%d, %q, %t), want (%d, %q, %t)", gotRule, gotRole, gotMatched, wantRule, wantRole, wantMatched)
			}
		})
	}
}

const encodedTestImage = `{"type":"input_image","image_url":"https://example.test/SECRET.png","file_id":"SECRET-id"}`

func encodedResultFixture(t testing.TB, inner string) ([]byte, []textSpan) {
	t.Helper()
	token, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	body := append([]byte(`{"output":`), token...)
	body = append(body, '}')
	var spans []textSpan
	appendToolResultStringSpan(&spans, gjson.ParseBytes(body).Get("output"), "tool", scopeSet{"tool": {}})
	return body, spans
}

func decodedResultOutput(t testing.TB, body []byte) string {
	t.Helper()
	var got struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body=%q err=%v", body, err)
	}
	return got.Output
}

func TestEncodedToolResultPreservesImageAndMachineBytes(t *testing.T) {
	for _, textType := range []string{"text", "input_text", "output_text"} {
		for _, action := range []string{"block", "strip", "obfs"} {
			t.Run(textType+"/"+action, func(t *testing.T) {
				inner := `[ {"type":"` + textType + `","text":"SECRET tail","id":"SECRET"}, ` + encodedTestImage + `, {"type":"future","text":"SECRET machine","number":1e+03} ]`
				body, spans := encodedResultFixture(t, inner)
				result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  "+action+": [SECRET]\n"))
				if err != nil || result.Invalid {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				if action == "block" {
					if result.Body != nil || result.Blocked == nil || *result.Blocked != (blockMatch{Term: "SECRET", Role: "tool"}) {
						t.Fatalf("result=%#v want tool block", result)
					}
					return
				}
				replacement := `" tail"`
				if action == "obfs" {
					replacement = `"S​ECRET tail"`
				}
				want := strings.Replace(inner, `"SECRET tail"`, replacement, 1)
				if result.Blocked != nil || decodedResultOutput(t, result.Body) != want {
					t.Fatalf("output=%q want=%q", decodedResultOutput(t, result.Body), want)
				}
			})
		}
	}
}

func TestEncodedToolResultRecognition(t *testing.T) {
	for _, tc := range []struct {
		name, inner string
		carrier     bool
	}{
		{"input url", `[{"type":"input_image","image_url":"SECRET"}]`, true},
		{"input file", `[{"type":"input_image","file_id":"SECRET"}]`, true},
		{"chat url", `[{"type":"image_url","image_url":{"url":"SECRET"}}]`, true},
		{"chat file", `[{"type":"image_url","image_url":{"file_id":"SECRET"}}]`, true},
		{"empty input url", `[{"type":"input_image","image_url":"","id":"SECRET"}]`, false},
		{"empty chat file", `[{"type":"image_url","image_url":{"file_id":""},"id":"SECRET"}]`, false},
		{"wrong input url", `[{"type":"input_image","image_url":{"url":"SECRET"}}]`, false},
		{"wrong chat url", `[{"type":"image_url","image_url":"SECRET"}]`, false},
		{"wrong file type", `[{"type":"input_image","file_id":123,"id":"SECRET"}]`, false},
		{"object", `{"type":"input_image","image_url":"SECRET"}`, false},
		{"primitive", `"SECRET"`, false},
		{"text array", `[{"type":"text","text":"SECRET"}]`, false},
		{"plain", `SECRET note`, false},
		{"invalid inner JSON", `[{"type":"input_image","image_url":"SECRET"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, spans := encodedResultFixture(t, tc.inner)
			if len(spans) != 1 || (spans[0].EncodedChildren != nil) != tc.carrier {
				t.Fatalf("spans=%#v want carrier=%t", spans, tc.carrier)
			}
			for _, action := range []string{"block", "strip", "obfs"} {
				_, current := encodedResultFixture(t, tc.inner)
				result, err := transformSelectedSpans(body, current, mustConfig(t, "words:\n  "+action+": [SECRET]\n"))
				if err != nil || result.Invalid {
					t.Fatalf("action=%s result=%#v err=%v", action, result, err)
				}
				if tc.carrier {
					if result.Body != nil || result.Blocked != nil {
						t.Fatalf("image-only carrier matched: %#v", result)
					}
				} else if action == "block" {
					if result.Blocked == nil || result.Blocked.Term != "SECRET" {
						t.Fatalf("ordinary string did not block: %#v", result)
					}
				} else {
					replacement := ""
					if action == "obfs" {
						replacement = "S​ECRET"
					}
					want := strings.ReplaceAll(tc.inner, "SECRET", replacement)
					if result.Blocked != nil || decodedResultOutput(t, result.Body) != want {
						t.Fatalf("result=%#v want output=%q", result, want)
					}
				}
			}
			var disabled []textSpan
			appendToolResultStringSpan(&disabled, gjson.ParseBytes(body).Get("output"), "tool", scopeSet{"user": {}})
			if len(disabled) != 0 {
				t.Fatalf("disabled role selected: %#v", disabled)
			}
		})
	}
}

func TestEncodedToolResultGlobalRuleOrder(t *testing.T) {
	for _, fold := range []bool{false, true} {
		for _, tc := range []struct {
			name, text, inner, terms, wantTerm, wantRole string
		}{
			{"rule-major", "FIRST", `[{"type":"text","text":"SECOND"},` + encodedTestImage + `]`, "FIRST, SECOND", "FIRST", "user"},
			{"earliest parent", "SECRET", `[{"type":"text","text":"SECRET"},` + encodedTestImage + `]`, "SECRET", "SECRET", "tool"},
			{"earliest ordinary", "SECRET", `[{"type":"text","text":"SECRET"},` + encodedTestImage + `]`, "SECRET", "SECRET", "user"},
			{"no cross-node", "RET", `[{"type":"text","text":"SEC"},{"type":"text","text":"RET"},` + encodedTestImage + `]`, "SECRET", "", ""},
		} {
			t.Run(fmt.Sprintf("%s/fold=%t", tc.name, fold), func(t *testing.T) {
				body, _ := encodedResultFixture(t, tc.inner)
				if tc.name == "earliest ordinary" {
					body = []byte(`{"machine":"` + strings.Repeat("z", 128) + `","text":"` + tc.text + `","output":` + benchmarkQuote(tc.inner) + `}`)
				} else {
					body = append(body[:len(body)-1], []byte(`,"text":"`+tc.text+`"}`)...)
				}
				root := gjson.ParseBytes(body)
				var spans []textSpan
				appendStringSpan(&spans, root.Get("text"), "user", scopeSet{"user": {}})
				appendToolResultStringSpan(&spans, root.Get("output"), "tool", scopeSet{"tool": {}})
				result, err := transformSelectedSpans(body, spans, mustConfig(t, fmt.Sprintf("words:\n  block: [%s]\nignore_case: %t\n", tc.terms, fold)))
				if err != nil || result.Invalid || result.Body != nil {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				if tc.wantTerm == "" {
					if result.Blocked != nil {
						t.Fatalf("cross-node/image match: %#v", result)
					}
				} else if result.Blocked == nil || *result.Blocked != (blockMatch{Term: tc.wantTerm, Role: tc.wantRole}) {
					t.Fatalf("blocked=%#v want=%s/%s", result.Blocked, tc.wantTerm, tc.wantRole)
				}
			})
		}
	}
}

func TestEncodedToolResultCancellationPreservesOuterToken(t *testing.T) {
	inner := `[{"type":"text","text":"a​b"},` + encodedTestImage + `]`
	body, _ := encodedResultFixture(t, inner)
	body = bytes.ReplaceAll(body, []byte("a​b"), []byte("a"+string([]byte{92})+"u200bb"))
	for _, otherChanged := range []bool{false, true} {
		t.Run(fmt.Sprintf("other=%t", otherChanged), func(t *testing.T) {
			current := append([]byte(nil), body...)
			if otherChanged {
				current = append(current[:len(current)-1], []byte(`,"text":"CHANGE"}`)...)
			}
			root := gjson.ParseBytes(current)
			var spans []textSpan
			appendToolResultStringSpan(&spans, root.Get("output"), "tool", scopeSet{"tool": {}})
			appendStringSpan(&spans, root.Get("text"), "user", scopeSet{"user": {}})
			result, err := transformSelectedSpans(current, spans, mustConfig(t, "words:\n  strip: ['​', CHANGE]\n  obfs: [ab]\n"))
			if err != nil || result.Invalid || result.Blocked != nil {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if !otherChanged {
				if result.Body != nil {
					t.Fatalf("canceled rewrite replaced body: %q", result.Body)
				}
			} else if want := bytes.Replace(current, []byte(`"CHANGE"`), []byte(`""`), 1); !bytes.Equal(result.Body, want) {
				t.Fatalf("body=%q want=%q", result.Body, want)
			}
		})
	}
}

func TestEncodedToolResultMultipleParentsAndEscapes(t *testing.T) {
	first := `[ {"type":"text","text":"aXb\n\"中文😀\""}, ` + encodedTestImage + ` ]`
	second := `[ {"type":"text","text":"cc"}, ` + encodedTestImage + ` ]`
	body := []byte(`{"first":` + benchmarkQuote(first) + `,"middle":"Xcc","second":` + benchmarkQuote(second) + `,"number":1e+03}`)
	root := gjson.ParseBytes(body)
	var spans []textSpan
	appendToolResultStringSpan(&spans, root.Get("second"), "tool", scopeSet{"tool": {}})
	appendStringSpan(&spans, root.Get("middle"), "user", scopeSet{"user": {}})
	appendToolResultStringSpan(&spans, root.Get("first"), "tool", scopeSet{"tool": {}})
	result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  strip: [X, ab]\n  obfs: [cc, cc]\n"))
	if err != nil || result.Invalid || result.Blocked != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	wantFirst := strings.Replace(first, `aXb`, ``, 1)
	wantSecond := strings.Replace(second, `"cc"`, `"c​c"`, 1)
	want := []byte(`{"first":` + benchmarkQuote(wantFirst) + `,"middle":"c​c","second":` + benchmarkQuote(wantSecond) + `,"number":1e+03}`)
	if !bytes.Equal(result.Body, want) {
		t.Fatalf("body=%q want=%q", result.Body, want)
	}
}

func TestEncodedToolResultDormantDescription(t *testing.T) {
	inner := `[{"type":"text","text":"CHANGE"},` + encodedTestImage + `]`
	body, _ := encodedResultFixture(t, inner)
	body = append(body[:len(body)-1], []byte(`,"tools":[{"type":"function","name":"f","description":"FLAT","function":{"description":"BLOCK"}}]}`)...)
	spans, err := selectTextSpans(body, "openai-response", scopeSet{"developer": {}})
	if err != nil {
		t.Fatal(err)
	}
	appendToolResultStringSpan(&spans, gjson.ParseBytes(body).Get("output"), "tool", scopeSet{"tool": {}})
	result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  block: [BLOCK]\n  strip: [FLAT, CHANGE]\n"))
	if err != nil || result.Invalid || result.Body != nil || result.Blocked == nil || *result.Blocked != (blockMatch{Term: "BLOCK", Role: "developer"}) {
		t.Fatalf("result=%#v err=%v want activated block", result, err)
	}
}

func TestEncodedToolResultMatcherPaths(t *testing.T) {
	t.Run("exact matcher rule order", func(t *testing.T) {
		var yaml strings.Builder
		yaml.WriteString("words:\n  block:\n")
		for i := 0; i < 256; i++ {
			fmt.Fprintf(&yaml, "    - TERM%03d\n", i)
		}
		inner := `[{"type":"text","text":"` + strings.Repeat("z", 16<<10) + `TERM200"},` + encodedTestImage + `]`
		body, _ := encodedResultFixture(t, inner)
		body = append(body[:len(body)-1], []byte(`,"text":"TERM100"}`)...)
		root := gjson.ParseBytes(body)
		var spans []textSpan
		appendToolResultStringSpan(&spans, root.Get("output"), "tool", scopeSet{"tool": {}})
		appendStringSpan(&spans, root.Get("text"), "user", scopeSet{"user": {}})
		result, err := transformSelectedSpans(body, spans, mustConfig(t, yaml.String()))
		if err != nil || result.Invalid || result.Body != nil || result.Blocked == nil || *result.Blocked != (blockMatch{Term: "TERM100", Role: "user"}) {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})
	for _, action := range []string{"strip", "obfs"} {
		for _, preflight := range []bool{false, true} {
			t.Run(fmt.Sprintf("folded KMP/%s/preflight=%t", action, preflight), func(t *testing.T) {
				terms := "ABCD"
				if preflight {
					terms += ", q0, q1, q2, q3, q4, q5, q6"
				}
				padding := strings.Repeat("z", 4<<10)
				inner := `[{"type":"text","text":"` + padding + `abcd"},{"type":"text","text":"` + padding + `"},` + encodedTestImage + `]`
				body, spans := encodedResultFixture(t, inner)
				cfg := mustConfig(t, "ignore_case: true\nwords:\n  "+action+": ["+terms+"]\n")
				result, err := transformSelectedSpans(body, spans, cfg)
				if err != nil || result.Invalid || result.Blocked != nil {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				replacement := ""
				if action == "obfs" {
					replacement = "a​bcd"
				}
				want := strings.Replace(inner, "abcd", replacement, 1)
				if decodedResultOutput(t, result.Body) != want {
					t.Fatalf("output=%q want=%q", decodedResultOutput(t, result.Body), want)
				}
			})
		}
	}
}

func TestEncodedToolResultValidatesEachCoordinateSpace(t *testing.T) {
	inner := `[{"type":"text","text":"SECRET"},` + encodedTestImage + `]`
	for _, mutate := range []struct {
		name string
		fn   func([]textSpan)
	}{
		{"outer range", func(spans []textSpan) { spans[0].RawEnd += 2 }},
		{"inner range", func(spans []textSpan) {
			spans[0].EncodedChildren = []textSpan{{RawStart: -1, RawEnd: 5, Text: "SECRET", Role: "tool"}}
		}},
		{"inner overlap", func(spans []textSpan) {
			spans[0].EncodedChildren = []textSpan{{RawStart: 23, RawEnd: 31, Text: "SECRET", Role: "tool"}, {RawStart: 24, RawEnd: 31, Text: "SECRET", Role: "tool"}}
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			body, spans := encodedResultFixture(t, inner)
			mutate.fn(spans)
			result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  block: [SECRET]\n"))
			if !errors.Is(err, errInvalidSpan) || result.Body != nil || result.Blocked != nil {
				t.Fatalf("result=%#v err=%v want invalid span before matching", result, err)
			}
		})
	}
}

func TestEncodedToolResultLogoRemainsDecodable(t *testing.T) {
	image, err := os.ReadFile("logo.png")
	if err != nil {
		t.Fatal(err)
	}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	for _, action := range []string{"block", "strip", "obfs"} {
		t.Run(action, func(t *testing.T) {
			inner := `[{"type":"text","text":"SECRET tail"},{"type":"input_image","image_url":` + benchmarkQuote(url) + `}]`
			body, spans := encodedResultFixture(t, inner)
			result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  "+action+": [iVBOR, SECRET]\n"))
			if err != nil || result.Invalid {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if action == "block" {
				if result.Blocked == nil || result.Blocked.Term != "SECRET" || result.Body != nil {
					t.Fatalf("image term blocked: %#v", result)
				}
				return
			}
			output := decodedResultOutput(t, result.Body)
			var parts []struct {
				ImageURL string `json:"image_url"`
			}
			if err := json.Unmarshal([]byte(output), &parts); err != nil || len(parts) != 2 || parts[1].ImageURL != url {
				t.Fatalf("image URL changed: err=%v", err)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(parts[1].ImageURL, "data:image/png;base64,"))
			if err != nil || !bytes.Equal(decoded, image) {
				t.Fatalf("PNG bytes changed: err=%v", err)
			}
			if _, err := png.Decode(bytes.NewReader(decoded)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEncodedToolResultChildStandardDecoding(t *testing.T) {
	u := string([]byte{92}) + "u"
	for _, surrogate := range []string{"d800", "dc00"} {
		t.Run(surrogate, func(t *testing.T) {
			raw := `"` + u + surrogate + u + `0065xample note"`
			var original string
			if err := json.Unmarshal([]byte(raw), &original); err != nil {
				t.Fatal(err)
			}
			inner := `[{"type":"text","text":` + raw + `},` + encodedTestImage + `]`
			body, spans := encodedResultFixture(t, inner)
			if len(spans) != 1 || len(spans[0].EncodedChildren) != 1 || spans[0].EncodedChildren[0].Text != original {
				t.Fatalf("spans=%#v want child text=%q", spans, original)
			}
			result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  strip: [note]\n"))
			if err != nil || result.Invalid || result.Blocked != nil {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			want := strings.Replace(inner, raw, benchmarkQuote(strings.TrimSuffix(original, "note")), 1)
			if decodedResultOutput(t, result.Body) != want {
				t.Fatalf("output=%q want=%q", decodedResultOutput(t, result.Body), want)
			}
		})
	}
}

func TestEncodedToolResultCanceledParentBesideChangedParent(t *testing.T) {
	first := `[{"type":"text","text":"a​b"},` + encodedTestImage + `]`
	second := `[{"type":"text","text":"CHANGE"},` + encodedTestImage + `]`
	firstToken := strings.Replace(benchmarkQuote(first), "a​b", "a"+string([]byte{92})+"u200bb", 1)
	body := []byte(`{"first":` + firstToken + `,"second":` + benchmarkQuote(second) + `}`)
	root := gjson.ParseBytes(body)
	var spans []textSpan
	appendToolResultStringSpan(&spans, root.Get("first"), "tool", scopeSet{"tool": {}})
	appendToolResultStringSpan(&spans, root.Get("second"), "tool", scopeSet{"tool": {}})
	result, err := transformSelectedSpans(body, spans, mustConfig(t, "words:\n  strip: ['​', CHANGE]\n  obfs: [ab]\n"))
	if err != nil || result.Invalid || result.Blocked != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	wantSecond := strings.Replace(second, `"CHANGE"`, `""`, 1)
	want := []byte(`{"first":` + firstToken + `,"second":` + benchmarkQuote(wantSecond) + `}`)
	if !bytes.Equal(result.Body, want) {
		t.Fatalf("body=%q want=%q", result.Body, want)
	}
}
