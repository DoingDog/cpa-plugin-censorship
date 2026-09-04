package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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
