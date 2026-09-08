package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestBeforeAuthBenchmarkFixtureResult(t *testing.T) {
	old := loadedSnapshot()
	t.Cleanup(func() { installSnapshot(old) })
	installSnapshot(mustBenchmarkConfig(t, "mode: block\nwords: [NEVER-MATCH]\n"))
	body := benchmarkScenarioBody("plain", "", 1000, "")
	request, err := json.Marshal(pluginapi.RequestInterceptRequest{RequestID: "benchmark", SourceFormat: "openai", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	got, err := handleMethod(pluginabi.MethodRequestInterceptBefore, request)
	if err != nil {
		t.Fatal(err)
	}
	var envelope pluginabi.Envelope
	if err := json.Unmarshal(got, &envelope); err != nil {
		t.Fatal(err)
	}
	wantResult, err := json.Marshal(pluginapi.RequestInterceptResponse{})
	if err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || !bytes.Equal(envelope.Result, wantResult) {
		t.Fatalf("before auth envelope = %q; want successful no-op response", got)
	}
}

func BenchmarkTransformMatrix(b *testing.B) {
	bodySizes := []int{1 << 10, 1 << 20, 20 << 20}
	wordCounts := []int{0, 1, 32, 256, 1024}
	for _, size := range bodySizes {
		for _, words := range wordCounts {
			for _, fold := range []bool{false, true} {
				name := fmt.Sprintf("body=%d/words=%d/fold=%t", size, words, fold)
				b.Run(name, func(b *testing.B) {
					body, cfg := benchmarkFixture(size, words, fold)
					got, err := transformRequest(body, "openai", cfg)
					if err != nil || got.Invalid || got.Blocked != nil || len(got.Body) != 0 {
						b.Fatalf("transformRequest() = %#v, %v; want unchanged no-op result", got, err)
					}
					b.ReportAllocs()
					b.SetBytes(int64(len(body)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						benchmarkTransformSink, benchmarkErrorSink = transformRequest(body, "openai", cfg)
					}
				})
			}
		}
	}
}

func BenchmarkFoldStripDense(b *testing.B) {
	text := strings.Repeat("a", 1<<20)
	rule := compiledRule{Term: "A", Runes: []rune("A")}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, matched := stripRule(text, rule, true)
		if !matched || len(got) != 0 {
			b.Fatalf("stripRule() matched=%t len=%d", matched, len(got))
		}
	}
}

func BenchmarkFoldObfuscateDense(b *testing.B) {
	text := strings.Repeat("a", 1<<20)
	rule := compiledRule{Term: "A", Runes: []rune("A")}
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, matched := obfuscateRule(text, rule, true, "⁠")
		if !matched || len(got) != len(text)*4 {
			b.Fatalf("obfuscateRule() matched=%t len=%d", matched, len(got))
		}
	}
}

func BenchmarkTransformScenarios(b *testing.B) {
	cases := []struct {
		name, yaml, text string
		nodes            int
		lastText         string
		excludedPosition string
		wantBlocked      *blockMatch
		wantBody         []byte
	}{
		{name: "mode=block/match=none/nodes=1", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1},
		{name: "mode=block/match=sparse/nodes=1000", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1000, lastText: "SECRET", wantBlocked: &blockMatch{Term: "SECRET", Role: "user"}},
		{name: "mode=strip/match=dense", yaml: "mode: strip\nwords: [SECRET]\n", text: strings.Repeat("SECRET", 128), nodes: 1, wantBody: benchmarkScenarioBody("", "", 1, "")},
		{name: "mode=strip/match=overlap", yaml: "mode: strip\nwords: [aa]\n", text: "aaa", nodes: 1, wantBody: benchmarkScenarioBody("a", "", 1, "")},
		{name: "mode=strip/match=cascade", yaml: "mode: strip\nwords: [AB, x]\n", text: "ABxABx", nodes: 1, wantBody: benchmarkScenarioBody("", "", 1, "")},
		{name: "mode=obfs/match=sparse", yaml: "mode: obfs\nwords: [SECRET]\n", text: "SECRET", nodes: 1, wantBody: benchmarkScenarioBody("S​ECRET", "", 1, "")},
		{name: "fold=ascii", yaml: "mode: strip\nignore_case: true\nwords: [Alpha]\n", text: "aLPHA", nodes: 1, wantBody: benchmarkScenarioBody("", "", 1, "")},
		{name: "fold=sigma", yaml: "mode: strip\nignore_case: true\nwords: [Σ]\n", text: "ςΣσ", nodes: 1, wantBody: benchmarkScenarioBody("", "", 1, "")},
		{name: "fold=kelvin", yaml: "mode: strip\nignore_case: true\nwords: [K]\n", text: "K", nodes: 1, wantBody: benchmarkScenarioBody("", "", 1, "")},
		{name: "fold=full-fold-miss", yaml: "mode: strip\nignore_case: true\nwords: [straße]\n", text: "STRASSE", nodes: 1},
		{name: "excluded=20MiB/before", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1, excludedPosition: "before"},
		{name: "excluded=20MiB/middle", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 2, excludedPosition: "middle"},
		{name: "excluded=20MiB/after", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1, excludedPosition: "after"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			cfg := mustBenchmarkConfig(b, tc.yaml)
			body := benchmarkScenarioBody(tc.text, tc.lastText, tc.nodes, tc.excludedPosition)
			got, err := transformRequest(body, "openai", cfg)
			if err != nil {
				b.Fatalf("transformRequest() error = %v", err)
			}
			if got.Invalid {
				b.Fatalf("transformRequest() Invalid = true; want false")
			}
			if (got.Blocked == nil) != (tc.wantBlocked == nil) || got.Blocked != nil && *got.Blocked != *tc.wantBlocked {
				b.Fatalf("transformRequest() Blocked = %#v; want %#v", got.Blocked, tc.wantBlocked)
			}
			if !bytes.Equal(got.Body, tc.wantBody) {
				b.Fatalf("transformRequest() Body = %q; want %q", got.Body, tc.wantBody)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkTransformSink, benchmarkErrorSink = transformRequest(body, "openai", cfg)
			}
		})
	}
}

func BenchmarkConcurrentSnapshotSwap(b *testing.B) {
	old := loadedSnapshot()
	b.Cleanup(func() { installSnapshot(old) })
	const yamlA = "mode: strip\nwords: [alpha]\n"
	const yamlB = "mode: obfs\nignore_case: true\nwords: [Beta]\n"
	installSnapshot(mustBenchmarkConfig(b, yamlA))
	configA := []byte(yamlA)
	configB := []byte(yamlB)
	requestA, err := json.Marshal(lifecycleRequest{ConfigYAML: &configA, SchemaVersion: pluginabi.SchemaVersion})
	if err != nil {
		b.Fatal(err)
	}
	requestB, err := json.Marshal(lifecycleRequest{ConfigYAML: &configB, SchemaVersion: pluginabi.SchemaVersion})
	if err != nil {
		b.Fatal(err)
	}
	body := []byte(`{"messages":[{"role":"user","content":"alpha BETA"}]}`)
	stop := make(chan struct{})
	stopped := make(chan struct{})
	reconfigureErr := make(chan error, 1)
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := handleMethod(pluginabi.MethodPluginReconfigure, requestA); err != nil {
					reconfigureErr <- err
					return
				}
				if _, err := handleMethod(pluginabi.MethodPluginReconfigure, requestB); err != nil {
					reconfigureErr <- err
					return
				}
			}
		}
	}()
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var result transformResult
		for pb.Next() {
			var err error
			result, err = transformRequest(body, "openai", loadedSnapshot())
			if err != nil {
				b.Error(err)
			}
			runtime.KeepAlive(result)
		}
	})
	b.StopTimer()
	close(stop)
	<-stopped
	select {
	case err := <-reconfigureErr:
		b.Fatal(err)
	default:
	}
}

func BenchmarkBeforeAuthRPCEnvelope(b *testing.B) {
	old := loadedSnapshot()
	b.Cleanup(func() { installSnapshot(old) })
	installSnapshot(mustBenchmarkConfig(b, "mode: block\nwords: [NEVER-MATCH]\n"))
	request, err := json.Marshal(pluginapi.RequestInterceptRequest{RequestID: "benchmark", SourceFormat: "openai", Body: benchmarkScenarioBody("plain", "", 1000, "")})
	if err != nil {
		b.Fatal(err)
	}
	b.Run("before_calls=1/after_calls=0", func(b *testing.B) {
		fixture, err := handleMethod(pluginabi.MethodRequestInterceptBefore, request)
		if err != nil {
			b.Fatal(err)
		}
		var envelope pluginabi.Envelope
		if err := json.Unmarshal(fixture, &envelope); err != nil {
			b.Fatal(err)
		}
		wantResult, err := json.Marshal(pluginapi.RequestInterceptResponse{})
		if err != nil {
			b.Fatal(err)
		}
		if !envelope.OK || !bytes.Equal(envelope.Result, wantResult) {
			b.Fatalf("before auth envelope = %q; want successful no-op response", fixture)
		}
		b.ReportAllocs()
		b.SetBytes(int64(len(request)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkEnvelopeSink, benchmarkErrorSink = handleMethod(pluginabi.MethodRequestInterceptBefore, request)
		}
	})
}

func mustBenchmarkConfig(b testing.TB, raw string) *configSnapshot {
	b.Helper()
	cfg, err := parseConfigYAML([]byte(raw))
	if err != nil {
		b.Fatal(err)
	}
	return cfg
}

func benchmarkFixture(size, wordCount int, fold bool) ([]byte, *configSnapshot) {
	rules := make([]compiledRule, wordCount)
	for i := range rules {
		term := fmt.Sprintf("term-%04d", i)
		rules[i] = compiledRule{Term: term}
	}
	cfg := benchmarkSnapshot(modeBlock, fold, rules)
	const prefix = `{"messages":[{"role":"user","content":"`
	const suffix = `"}]}`
	if size < len(prefix)+len(suffix) {
		panic("benchmark body size too small")
	}
	body := []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
	return body, cfg
}

func benchmarkQuote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func benchmarkScenarioBody(text, lastText string, nodes int, excludedPosition string) []byte {
	if nodes < 1 {
		nodes = 1
	}
	messages := make([]string, 0, nodes+1)
	for i := 0; i < nodes; i++ {
		nodeText := text
		if lastText != "" && i == nodes-1 {
			nodeText = lastText
		}
		messages = append(messages, `{"role":"user","content":`+benchmarkQuote(nodeText)+`}`)
	}
	if excludedPosition != "" {
		payload := strings.Repeat("SECRET", (20<<20)/len("SECRET")+1)[:20<<20]
		image := `{"role":"user","content":[{"type":"image_url","image_url":{"url":` + benchmarkQuote(payload) + `}}]}`
		switch excludedPosition {
		case "before":
			messages = append([]string{image}, messages...)
		case "middle":
			at := len(messages) / 2
			messages = append(messages, "")
			copy(messages[at+1:], messages[at:])
			messages[at] = image
		case "after":
			messages = append(messages, image)
		default:
			panic("unknown excluded position")
		}
	}
	return []byte(`{"messages":[` + strings.Join(messages, ",") + `]}`)
}

func BenchmarkDuplicateValidation(b *testing.B) {
	runBenchmarkDuplicateValidation(b)
}

func BenchmarkTextPartScanning(b *testing.B) {
	runBenchmarkTextPartScanning(b)
}

func BenchmarkDisabledRoleSelectors(b *testing.B) {
	runBenchmarkDisabledRoleSelectors(b)
}

func BenchmarkGeminiSystemOnly(b *testing.B) {
	const content = `{"role":"user","parts":[{"text":"hidden"}]}`
	contents := strings.TrimSuffix(strings.Repeat(content+",", 1<<14), ",")
	body := []byte(`{"systemInstruction":{"parts":[{"text":"system"}]},"contents":[` + contents + `]}`)
	roles := scopeSet{"system": {}}
	spans, err := selectTextSpans(body, "gemini", roles)
	if err != nil || len(spans) != 1 || spans[0].Text != "system" || spans[0].Role != "system" {
		b.Fatalf("selectTextSpans() = %#v, %v; want one system span", spans, err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSpansSink, benchmarkErrorSink = selectTextSpans(body, "gemini", roles)
	}
}

func BenchmarkEmptyStringSpans(b *testing.B) {
	body := benchmarkScenarioBody("", "", 1000, "")
	roles := scopeSet{"user": {}}
	spans, err := selectTextSpans(body, "openai", roles)
	if err != nil || len(spans) != 0 {
		b.Fatalf("selectTextSpans() = %#v, %v; want no empty string spans", spans, err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSpansSink, benchmarkErrorSink = selectTextSpans(body, "openai", roles)
	}
}

func BenchmarkRebuildChangedSpans(b *testing.B) {
	runBenchmarkRebuildChangedSpans(b)
}

func BenchmarkSuccessEnvelope(b *testing.B) {
	runBenchmarkSuccessEnvelope(b)
}

func BenchmarkExactRewriteStrategies(b *testing.B) {
	runBenchmarkExactRewriteStrategies(b)
}

func BenchmarkExactRewritePreflight(b *testing.B) {
	runBenchmarkExactRewritePreflight(b)
}

func TestBenchmarkExactRewritePreflightMatchesBaseline(t *testing.T) {
	rules := func(terms ...string) []compiledRule {
		out := make([]compiledRule, len(terms))
		for i, term := range terms {
			out[i] = compiledRule{Term: term}
		}
		return out
	}
	cases := []struct {
		name            string
		rules           []compiledRule
		spans           []textSpan
		wantSpanChanged []bool
		wantChanged     bool
	}{
		{
			name:            "none multiple spans",
			rules:           rules("FIRST", "MIDDLE", "LAST", "DUP", "DUP", "AB", "ab", "CROSS"),
			spans:           []textSpan{{Text: "plain", Role: "user"}, {Text: "also plain", Role: "assistant"}},
			wantSpanChanged: []bool{false, false},
		},
		{name: "first", rules: rules("FIRST", "MIDDLE", "LAST"), spans: []textSpan{{Text: "FIRST plain", Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
		{name: "middle", rules: rules("FIRST", "MIDDLE", "LAST"), spans: []textSpan{{Text: "plain MIDDLE plain", Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
		{name: "last", rules: rules("FIRST", "MIDDLE", "LAST"), spans: []textSpan{{Text: "plain LAST", Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
		{name: "dense", rules: rules("DUP"), spans: []textSpan{{Text: strings.Repeat("DUP", 32), Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
		{name: "duplicate", rules: rules("DUP", "DUP"), spans: []textSpan{{Text: "DUP", Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
		{name: "cross span", rules: rules("CROSS"), spans: []textSpan{{Text: "CR", Role: "user"}, {Text: "OSS", Role: "assistant"}}, wantSpanChanged: []bool{false, false}},
		{name: "later span hit", rules: rules("LATER"), spans: []textSpan{{Text: "plain", Role: "user"}, {Text: "LATER plain", Role: "assistant"}}, wantSpanChanged: []bool{false, true}, wantChanged: true},
		{name: "strip cascade", rules: rules("AB", "ab"), spans: []textSpan{{Text: "aABb", Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
		{name: "obfs cascade", rules: rules("AB", "A​B"), spans: []textSpan{{Text: "AB", Role: "user"}}, wantSpanChanged: []bool{true}, wantChanged: true},
	}
	for _, selectedMode := range []mode{modeStrip, modeObfs} {
		for _, tc := range cases {
			t.Run(string(selectedMode)+"/"+tc.name, func(t *testing.T) {
				cfg := benchmarkSnapshot(selectedMode, false, append([]compiledRule(nil), tc.rules...))
				baseline := append([]textSpan(nil), tc.spans...)
				candidate := append([]textSpan(nil), tc.spans...)
				_, baselineChanged := applyMode(baseline, cfg)
				if baselineChanged != tc.wantChanged {
					t.Fatalf("baseline changed = %t, want %t", baselineChanged, tc.wantChanged)
				}
				for i, wantSpanChanged := range tc.wantSpanChanged {
					if baseline[i].Changed != wantSpanChanged {
						t.Fatalf("baseline span %d changed = %t, want %t", i, baseline[i].Changed, wantSpanChanged)
					}
				}
				matcher := newByteMatcher(cfg.Rules[cfg.BlockEnd:], cfg.BlockEnd)
				gotChanged := benchmarkExactRewriteSpansStrategy(candidate, cfg, matcher)
				if gotChanged != tc.wantChanged {
					t.Fatalf("changed = %t, want %t", gotChanged, tc.wantChanged)
				}
				for i := range baseline {
					if candidate[i].Text != baseline[i].Text || candidate[i].Changed != baseline[i].Changed {
						t.Errorf("span %d = %#v, want %#v", i, candidate[i], baseline[i])
					}
				}
			})
		}
	}
}

func BenchmarkFoldRootTransitions(b *testing.B) {
	runBenchmarkFoldRootTransitions(b)
}

func BenchmarkRewritePreflightStrategies(b *testing.B) {
	runBenchmarkRewritePreflightStrategies(b)
}

func BenchmarkExactBlockStrategies(b *testing.B) {
	runBenchmarkExactBlockStrategies(b)
}

func BenchmarkFoldedRewriteStrategies(b *testing.B) {
	runBenchmarkFoldedRewriteStrategies(b)
}

func BenchmarkMixedTransformScenario(b *testing.B) {
	rules := []compiledRule{{Term: "BLOCK"}, {Term: "AB"}, {Term: "xy"}}
	cfg := benchmarkSnapshot(modeBlock, false, rules)
	cfg.BlockEnd = 1
	cfg.StripEnd = 2
	cfg.rangesSet = true
	if err := compileSnapshot(cfg); err != nil {
		b.Fatal(err)
	}
	body := benchmarkScenarioBody("ABxy", "", 1, "")
	want := benchmarkScenarioBody("x​y", "", 1, "")
	got, err := transformRequest(body, "openai", cfg)
	if err != nil || got.Invalid || got.Blocked != nil || !bytes.Equal(got.Body, want) {
		b.Fatalf("transformRequest() = %#v, %v; want mixed cascade body %q", got, err, want)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkTransformSink, benchmarkErrorSink = transformRequest(body, "openai", cfg)
	}
}

var (
	benchmarkBoolSink      bool
	benchmarkBytesSink     []byte
	benchmarkEnvelopeSink  []byte
	benchmarkErrorSink     error
	benchmarkFoldRuleSink  int
	benchmarkSpansSink     []textSpan
	benchmarkStringSink    string
	benchmarkTransformSink transformResult
)

func runBenchmarkDuplicateValidation(b *testing.B) {
	body := []byte(`{"messages":[{"role":"user","content":"` + strings.Repeat("x", 1<<12) + `"}],"metadata":{"request_id":"one","request_id":"two"}}`)
	roles := scopeSet{"user": {}}
	if spans, err := selectTextSpans(body, "openai", roles); !errors.Is(err, errInvalidRequest) || spans != nil {
		b.Fatalf("selectTextSpans() = %#v, %v; want invalid duplicate request", spans, err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSpansSink, benchmarkErrorSink = selectTextSpans(body, "openai", roles)
	}
}

func runBenchmarkTextPartScanning(b *testing.B) {
	text := strings.Repeat("x", 1<<14)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":` + benchmarkQuote(text) + `},{"text":"ignored","functionCall":{"name":"noop"}},{"text":"tail"}]},{"role":"model","parts":[{"text":"assistant"}]}]}`)
	roles := scopeSet{"user": {}}
	spans, err := selectTextSpans(body, "gemini", roles)
	if err != nil || len(spans) != 2 || spans[0].Text != text || spans[1].Text != "tail" {
		b.Fatalf("selectTextSpans() = %#v, %v; want two allowed user text parts", spans, err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSpansSink, benchmarkErrorSink = selectTextSpans(body, "gemini", roles)
	}
}

func runBenchmarkDisabledRoleSelectors(b *testing.B) {
	body := []byte(`{"messages":[{"role":"user","content":"hidden"},{"role":"assistant","content":"also hidden"}]}`)
	roles := scopeSet{"system": {}}
	spans, err := selectTextSpans(body, "openai", roles)
	if err != nil || len(spans) != 0 {
		b.Fatalf("selectTextSpans() = %#v, %v; want no spans for disabled roles", spans, err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSpansSink, benchmarkErrorSink = selectTextSpans(body, "openai", roles)
	}
}

func runBenchmarkRebuildChangedSpans(b *testing.B) {
	first := strings.Repeat("prefix ", 512) + "BLOCKME"
	body := []byte(`{"messages":[{"role":"user","content":` + benchmarkQuote(first) + `},{"role":"user","content":"BLOCKME"}]}`)
	spans, err := selectTextSpans(body, "openai", scopeSet{"user": {}})
	if err != nil || len(spans) != 2 {
		b.Fatalf("selectTextSpans() = %#v, %v; want two user spans", spans, err)
	}
	spans[0].Text = strings.TrimSuffix(first, "BLOCKME")
	spans[0].Changed = true
	spans[1].Text = ""
	spans[1].Changed = true
	want := []byte(`{"messages":[{"role":"user","content":` + benchmarkQuote(spans[0].Text) + `},{"role":"user","content":""}]}`)
	got, err := rebuildBody(body, spans)
	if err != nil || !bytes.Equal(got, want) {
		b.Fatalf("rebuildBody() = %q, %v; want %q", got, err, want)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkBytesSink, benchmarkErrorSink = rebuildBody(body, spans)
	}
}

func runBenchmarkSuccessEnvelope(b *testing.B) {
	value := pluginapi.RequestInterceptResponse{Body: []byte(`{"messages":[]}`)}
	wantResult, err := json.Marshal(value)
	if err != nil {
		b.Fatal(err)
	}
	got, err := okEnvelope(value)
	var envelope pluginabi.Envelope
	if err != nil || json.Unmarshal(got, &envelope) != nil || !envelope.OK || !bytes.Equal(envelope.Result, wantResult) {
		b.Fatalf("okEnvelope() = %q, %v; want successful response envelope", got, err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(got)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkEnvelopeSink, benchmarkErrorSink = okEnvelope(value)
	}
}

func runBenchmarkExactRewriteStrategies(b *testing.B) {
	type benchmarkCase struct {
		mode      mode
		rules     int
		textBytes int
		match     string
		set       string
	}
	var cases []benchmarkCase
	for _, selectedMode := range []mode{modeStrip, modeObfs} {
		for _, ruleCount := range []int{32, 128, 256, 1024} {
			for _, textBytes := range []int{4 << 10, 64 << 10, 1 << 20} {
				for _, match := range []string{"none", "first", "middle", "last", "sparse", "dense", "cascade"} {
					set := "holdout"
					if ruleCount == 32 || textBytes == 4<<10 {
						set = "calibration"
					}
					cases = append(cases, benchmarkCase{
						mode: selectedMode, rules: ruleCount, textBytes: textBytes,
						match: match, set: set,
					})
				}
			}
		}
	}

	const target = "BLOCKME"
	for _, tc := range cases {
		tc := tc
		rules := benchmarkRules(tc.rules, target)
		text := ""
		if tc.match == "cascade" {
			rules[0] = compiledRule{Term: "AB"}
			rules[1] = compiledRule{Term: "x"}
			text = strings.Repeat("ABx", (tc.textBytes/3)+1)[:tc.textBytes]
		} else if tc.match == "none" {
			text = strings.Repeat("x", tc.textBytes)
		} else if tc.match == "dense" {
			text = strings.Repeat(target, (tc.textBytes/len(target))+1)[:tc.textBytes]
		} else if tc.match == "sparse" {
			text = benchmarkSizedText(tc.textBytes, target)
		} else {
			text = benchmarkPositionedText(tc.textBytes, target, tc.match)
		}
		cfg := benchmarkSnapshot(tc.mode, false, rules)
		wantText := benchmarkRewriteExpected(text, rules, tc.mode, cfg.ObfsChar)
		body := benchmarkScenarioBody(text, "", 1, "")
		want := benchmarkScenarioBody(wantText, "", 1, "")
		name := benchmarkBaselineName(tc.mode, tc.rules, tc.textBytes, "literal", tc.match, tc.set)
		b.Run(name, func(b *testing.B) {
			got, err := transformRequest(body, "openai", cfg)
			if err != nil || got.Invalid || got.Blocked != nil {
				b.Fatalf("transformRequest() = %#v, %v; want rewrite result", got, err)
			}
			if tc.match == "none" {
				if len(got.Body) != 0 {
					b.Fatalf("all-miss body length = %d; want 0", len(got.Body))
				}
			} else if !bytes.Equal(got.Body, want) {
				b.Fatalf("transformRequest() body = %q; want %q", got.Body, want)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkTransformSink, benchmarkErrorSink = transformRequest(body, "openai", cfg)
			}
		})
	}
}

func benchmarkRewriteExpected(text string, rules []compiledRule, selectedMode mode, obfsChar string) string {
	for _, rule := range rules {
		replacement := ""
		if selectedMode == modeObfs {
			firstRune, firstWidth := utf8.DecodeRuneInString(rule.Term)
			if firstRune == utf8.RuneError && firstWidth == 0 {
				panic("benchmark rule must not be empty")
			}
			replacement = rule.Term[:firstWidth] + obfsChar + rule.Term[firstWidth:]
		}
		if selectedMode == modeStrip {
			text = strings.ReplaceAll(text, rule.Term, "")
		} else {
			text = strings.ReplaceAll(text, rule.Term, replacement)
		}
	}
	return text
}

func runBenchmarkExactRewritePreflight(b *testing.B) {
	const (
		ruleCount = 1024
		textBytes = 20 << 20
	)
	type benchmarkCase struct {
		match string
	}
	cases := []benchmarkCase{
		{match: "total-miss"},
		{match: "first-sparse"},
		{match: "last-sparse"},
		{match: "dense"},
		{match: "duplicate"},
		{match: "cross-span"},
		{match: "cascade"},
	}
	for _, selectedMode := range []mode{modeStrip, modeObfs} {
		for _, tc := range cases {
			tc := tc
			rules, texts := benchmarkExactRewriteFixture(selectedMode, tc.match, ruleCount, textBytes)
			cfg := benchmarkSnapshot(selectedMode, false, rules)
			wantSpans := benchmarkExactRewriteSpans(texts)
			_, wantChanged := applyMode(wantSpans, cfg)
			for _, impl := range []struct {
				name    string
				matcher *byteMatcher
			}{
				{name: "baseline"},
				{name: "preflight", matcher: newByteMatcher(cfg.Rules[cfg.BlockEnd:], cfg.BlockEnd)},
			} {
				impl := impl
				for _, parallel := range []bool{false, true} {
					parallel := parallel
					execution := "serial"
					if parallel {
						execution = "parallel"
					}
					name := fmt.Sprintf("impl=%s/mode=%s/rules=%d/text=%d/pattern=literal/match=%s/set=holdout/execution=%s", impl.name, selectedMode, ruleCount, textBytes, tc.match, execution)
					b.Run(name, func(b *testing.B) {
						gotSpans := benchmarkExactRewriteSpans(texts)
						gotChanged := benchmarkExactRewriteSpansStrategy(gotSpans, cfg, impl.matcher)
						if gotChanged != wantChanged {
							b.Fatalf("strategy changed = %t; want %t", gotChanged, wantChanged)
						}
						for i := range wantSpans {
							if gotSpans[i].Text != wantSpans[i].Text || gotSpans[i].Changed != wantSpans[i].Changed {
								b.Fatalf("span %d = %#v; want %#v", i, gotSpans[i], wantSpans[i])
							}
						}

						b.ReportAllocs()
						b.SetBytes(textBytes)
						b.ResetTimer()
						if parallel {
							b.RunParallel(func(pb *testing.PB) {
								var text string
								var changed bool
								for pb.Next() {
									if len(texts) == 1 {
										text, changed = benchmarkExactRewriteStrategy(texts[0], cfg, impl.matcher)
									} else {
										spans := [...]textSpan{{Text: texts[0], Role: "user"}, {Text: texts[1], Role: "assistant"}}
										changed = benchmarkExactRewriteSpansStrategy(spans[:], cfg, impl.matcher)
										text = spans[0].Text
									}
								}
								runtime.KeepAlive(text)
								runtime.KeepAlive(changed)
							})
							return
						}
						for i := 0; i < b.N; i++ {
							if len(texts) == 1 {
								benchmarkStringSink, benchmarkBoolSink = benchmarkExactRewriteStrategy(texts[0], cfg, impl.matcher)
								continue
							}
							spans := [...]textSpan{{Text: texts[0], Role: "user"}, {Text: texts[1], Role: "assistant"}}
							benchmarkBoolSink = benchmarkExactRewriteSpansStrategy(spans[:], cfg, impl.matcher)
							benchmarkStringSink = spans[0].Text
						}
					})
				}
			}
		}
	}
}

func benchmarkExactRewriteFixture(selectedMode mode, match string, ruleCount, textBytes int) ([]compiledRule, []string) {
	rules := benchmarkRules(ruleCount, "")
	setRule := func(index int, term string) {
		rules[index] = compiledRule{Term: term}
	}
	switch match {
	case "total-miss":
		return rules, []string{strings.Repeat("x", textBytes)}
	case "first-sparse":
		setRule(0, "FIRSTHIT")
		return rules, []string{benchmarkPositionedText(textBytes, "FIRSTHIT", "first")}
	case "last-sparse":
		setRule(len(rules)-1, "LASTHIT")
		return rules, []string{benchmarkPositionedText(textBytes, "LASTHIT", "last")}
	case "dense":
		setRule(0, "DENSEHIT")
		return rules, []string{strings.Repeat("DENSEHIT", textBytes/len("DENSEHIT")+1)[:textBytes]}
	case "duplicate":
		setRule(len(rules)/2, "DUPHIT")
		setRule(len(rules)/2+1, "DUPHIT")
		return rules, []string{benchmarkPositionedText(textBytes, "DUPHIT", "middle")}
	case "cross-span":
		setRule(len(rules)-1, "CROSSHIT")
		firstBytes := textBytes / 2
		return rules, []string{
			strings.Repeat("x", firstBytes-len("CRO")) + "CRO",
			"SSHIT" + strings.Repeat("x", textBytes-firstBytes-len("SSHIT")),
		}
	case "cascade":
		if selectedMode == modeStrip {
			setRule(0, "AB")
			setRule(1, "ab")
			return rules, []string{benchmarkSizedText(textBytes, "aABb")}
		}
		setRule(0, "AB")
		setRule(1, "A​B")
		return rules, []string{benchmarkSizedText(textBytes, "AB")}
	default:
		panic("unknown exact rewrite benchmark match")
	}
}

func benchmarkExactRewriteSpans(texts []string) []textSpan {
	spans := make([]textSpan, len(texts))
	for i, text := range texts {
		spans[i] = textSpan{Text: text, Role: "user"}
		if i > 0 {
			spans[i].Role = "assistant"
		}
	}
	return spans
}

func benchmarkExactRewriteStrategy(text string, cfg *configSnapshot, matcher *byteMatcher) (string, bool) {
	spans := [...]textSpan{{Text: text, Role: "user"}}
	if matcher != nil {
		if _, matched := matcher.match(text); !matched {
			return text, false
		}
	}
	_, changed := applyMode(spans[:], cfg)
	return spans[0].Text, changed
}

func benchmarkExactRewriteSpansStrategy(spans []textSpan, cfg *configSnapshot, matcher *byteMatcher) bool {
	changed := false
	for i := range spans {
		text, spanChanged := benchmarkExactRewriteStrategy(spans[i].Text, cfg, matcher)
		spans[i].Text = text
		if spanChanged {
			spans[i].Changed = true
			changed = true
		}
	}
	return changed
}

func runBenchmarkFoldRootTransitions(b *testing.B) {
	cases := []struct {
		rules, text int
		set         string
	}{
		{rules: 32, text: 4 << 10, set: "calibration"},
		{rules: 128, text: 64 << 10, set: "holdout"},
	}
	for _, tc := range cases {
		tc := tc
		b.Run(benchmarkBaselineName(modeBlock, tc.rules, tc.text, "fold-root", "none", tc.set), func(b *testing.B) {
			matcher := benchmarkSnapshot(modeBlock, true, benchmarkFoldTransitionRules(tc.rules)).BlockMatcher
			text := strings.Repeat("az", tc.text/2)
			if rule, matched := matcher.match(text); matched || rule != -1 {
				b.Fatalf("matcher.match() = %d, %t; want no match", rule, matched)
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(text)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkFoldRuleSink, benchmarkBoolSink = matcher.match(text)
			}
		})
	}
}

func runBenchmarkRewritePreflightStrategies(b *testing.B) {
	cases := []struct {
		rules, text int
		set         string
		mixed       bool
	}{
		{rules: 8, text: 4 << 10, set: "calibration"},
		{rules: 32, text: 16 << 10, set: "calibration"},
		{rules: 32, text: 16 << 10, set: "mixed-holdout", mixed: true},
		{rules: 128, text: 64 << 10, set: "holdout"},
	}
	strategies := []struct {
		name      string
		preflight bool
	}{
		{name: "baseline"},
		{name: "preflight", preflight: true},
	}
	for _, tc := range cases {
		tc := tc
		rules := benchmarkRules(tc.rules, "")
		selectedMode := modeStrip
		pattern := "folded"
		if tc.mixed {
			rules = append([]compiledRule{{Term: "BLOCKME"}}, rules...)
			selectedMode = modeBlock
			pattern = "mixed-folded"
		}
		cfg := benchmarkSnapshot(selectedMode, true, rules)
		if tc.mixed {
			cfg.BlockEnd = 1
			cfg.StripEnd = len(cfg.Rules)
			cfg.rangesSet = true
			if err := compileSnapshot(cfg); err != nil {
				b.Fatal(err)
			}
		}
		text := benchmarkSizedText(tc.text, "")
		for _, strategy := range strategies {
			strategy := strategy
			b.Run(benchmarkStrategyName(strategy.name, selectedMode, len(rules), tc.text, pattern, "none", tc.set), func(b *testing.B) {
				if benchmarkFoldRewriteStrategy(text, cfg, strategy.preflight) {
					b.Fatal("total-miss strategy reported a match")
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					benchmarkBoolSink = benchmarkFoldRewriteStrategy(text, cfg, strategy.preflight)
				}
			})
		}
	}
}

func TestBenchmarkFoldRewriteStrategyPreservesMixedBlockSemantics(t *testing.T) {
	cfg := benchmarkSnapshot(modeBlock, true, []compiledRule{{Term: "BLOCKME"}, {Term: "rewrite"}})
	cfg.BlockEnd = 1
	cfg.StripEnd = len(cfg.Rules)
	cfg.rangesSet = true
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}

	withPreflight := benchmarkFoldRewriteStrategy("blockme", cfg, true)
	withoutPreflight := benchmarkFoldRewriteStrategy("blockme", cfg, false)
	if withPreflight != withoutPreflight {
		t.Fatalf("mixed block results differ: preflight=%t baseline=%t", withPreflight, withoutPreflight)
	}
	if withPreflight {
		t.Fatal("mixed block strategy reported a rewrite")
	}
}

func benchmarkFoldRewriteStrategy(text string, cfg *configSnapshot, preflight bool) bool {
	if !preflight {
		withoutPreflight := *cfg
		withoutPreflight.RewriteMatcher = nil
		cfg = &withoutPreflight
	}
	spans := [...]textSpan{{Text: text}}
	_, changed := applyMode(spans[:], cfg)
	return changed
}

func TestBenchmarkPositionedText(t *testing.T) {
	const size = 32
	const match = "BLOCKME"
	tests := []struct {
		position string
		want     int
	}{
		{position: "first", want: 0},
		{position: "middle", want: (size - len(match)) / 2},
		{position: "last", want: size - len(match)},
	}
	for _, test := range tests {
		text := benchmarkPositionedText(size, match, test.position)
		if len(text) != size {
			t.Fatalf("%s text length = %d, want %d", test.position, len(text), size)
		}
		if got := strings.Index(text, match); got != test.want {
			t.Fatalf("%s match index = %d, want %d", test.position, got, test.want)
		}
	}
}

func runBenchmarkExactBlockStrategies(b *testing.B) {
	cases := []struct {
		rules, text, target int
		match, set          string
		tune                bool
	}{
		{rules: 32, text: 4 << 10, target: 31, match: "last", set: "holdout"},
		{rules: 128, text: 64 << 10, target: 127, match: "last", set: "holdout"},
		{rules: 256, text: 16 << 10, target: 255, match: "last", set: "calibration", tune: true},
		{rules: 256, text: 64 << 10, target: 255, match: "last", set: "calibration", tune: true},
		{rules: 256, text: 16 << 10, target: 0, match: "first", set: "holdout"},
		{rules: 256, text: 16 << 10, target: 128, match: "middle", set: "holdout"},
	}
	strategies := []struct {
		name       string
		prefix     int
		production bool
	}{
		{name: "baseline", prefix: -1},
		{name: "ac-prefix-0", prefix: 0},
		{name: "ac-prefix-4", prefix: 4},
		{name: "ac-prefix-8", prefix: 8},
		{name: "ac-prefix-16", prefix: 16},
		{name: "production", production: true},
	}
	const target = "BLOCKME"
	for _, tc := range cases {
		tc := tc
		rules := benchmarkRules(tc.rules, "")
		rules[tc.target] = compiledRule{Term: target}
		text := benchmarkPositionedText(tc.text, target, tc.match)
		cfg := benchmarkSnapshot(modeBlock, false, append([]compiledRule(nil), rules...))
		spans := [...]textSpan{{Text: text, Role: "user"}}
		for _, strategy := range strategies {
			strategy := strategy
			if !tc.tune && strategy.name != "baseline" && !strategy.production {
				continue
			}
			b.Run(benchmarkStrategyName(strategy.name, modeBlock, tc.rules, tc.text, "literal", tc.match, tc.set), func(b *testing.B) {
				var matcher *byteMatcher
				if !strategy.production && strategy.prefix >= 0 {
					matcher = newByteMatcher(rules[strategy.prefix:], strategy.prefix)
				}
				run := func() (int, bool) {
					if strategy.production {
						ruleIndex, _, matched := matchExactBlock(spans[:], cfg)
						return ruleIndex, matched
					}
					return benchmarkExactBlockStrategy(text, rules, strategy.prefix, matcher)
				}
				if got, matched := run(); !matched || got != tc.target {
					b.Fatalf("strategy match = %d, %t; want %d, true", got, matched, tc.target)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					benchmarkFoldRuleSink, benchmarkBoolSink = run()
				}
			})
		}
	}
}

func benchmarkExactBlockStrategy(text string, rules []compiledRule, prefix int, matcher *byteMatcher) (int, bool) {
	if prefix < 0 {
		for ruleIndex, rule := range rules {
			if strings.Contains(text, rule.Term) {
				return ruleIndex, true
			}
		}
		return -1, false
	}
	if prefix > len(rules) {
		prefix = len(rules)
	}
	for ruleIndex := 0; ruleIndex < prefix; ruleIndex++ {
		if strings.Contains(text, rules[ruleIndex].Term) {
			return ruleIndex, true
		}
	}
	return matcher.match(text)
}

func runBenchmarkFoldedRewriteStrategies(b *testing.B) {
	type benchmarkCase struct {
		mode          mode
		text, scalars int
		script, match string
		set           string
		tune          bool
	}
	var cases []benchmarkCase
	for _, scalars := range []int{4, 8, 16} {
		for _, textBytes := range []int{4 << 10, 16 << 10, 64 << 10} {
			cases = append(cases, benchmarkCase{
				mode: modeStrip, text: textBytes, scalars: scalars,
				script: "ascii", match: "none", set: "calibration", tune: true,
			})
		}
	}
	cases = append(cases,
		benchmarkCase{mode: modeObfs, text: 64 << 10, scalars: 16, script: "ascii", match: "none", set: "holdout"},
		benchmarkCase{mode: modeStrip, text: 64 << 10, scalars: 16, script: "ascii", match: "sparse", set: "holdout"},
		benchmarkCase{mode: modeObfs, text: 64 << 10, scalars: 16, script: "ascii", match: "sparse", set: "holdout"},
		benchmarkCase{mode: modeStrip, text: 64 << 10, scalars: 16, script: "ascii", match: "dense", set: "holdout"},
		benchmarkCase{mode: modeObfs, text: 64 << 10, scalars: 16, script: "ascii", match: "dense", set: "holdout"},
		benchmarkCase{mode: modeStrip, text: 64 << 10, scalars: 16, script: "ascii", match: "overlap", set: "holdout"},
		benchmarkCase{mode: modeObfs, text: 64 << 10, scalars: 16, script: "sigma", match: "sparse", set: "holdout"},
		benchmarkCase{mode: modeStrip, text: 64 << 10, scalars: 16, script: "kelvin", match: "sparse", set: "holdout"},
		benchmarkCase{mode: modeObfs, text: 64 << 10, scalars: 16, script: "invalid-utf8", match: "sparse", set: "holdout"},
		benchmarkCase{mode: modeStrip, text: 64 << 10, scalars: 3, script: "ascii", match: "none", set: "holdout"},
		benchmarkCase{mode: modeStrip, text: (4 << 10) - 1, scalars: 16, script: "ascii", match: "none", set: "holdout"},
	)
	strategies := []struct {
		name       string
		kmp        bool
		production bool
	}{
		{name: "baseline"},
		{name: "kmp", kmp: true},
		{name: "production", production: true},
	}
	for _, tc := range cases {
		tc := tc
		text, term := benchmarkFoldedRewriteInput(tc.text, tc.scalars, tc.script, tc.match)
		cfg := benchmarkSnapshot(tc.mode, true, []compiledRule{{Term: term}})
		rule := cfg.Rules[0]
		want, wantMatched := rewriteFolded(text, rule.Runes, cfg.ObfsChar, tc.mode == modeObfs)
		for _, strategy := range strategies {
			strategy := strategy
			if !tc.tune && strategy.kmp {
				continue
			}
			name := benchmarkStrategyName(
				strategy.name,
				tc.mode,
				1,
				tc.text,
				fmt.Sprintf("%s-%dscalars", tc.script, tc.scalars),
				tc.match,
				tc.set,
			)
			b.Run(name, func(b *testing.B) {
				run := func() (string, bool) {
					switch {
					case strategy.production && tc.mode == modeObfs:
						return obfuscateRule(text, rule, true, cfg.ObfsChar)
					case strategy.production:
						return stripRule(text, rule, true)
					case strategy.kmp:
						return rewriteFoldedKMP(text, rule, cfg.ObfsChar, tc.mode == modeObfs)
					default:
						return rewriteFolded(text, rule.Runes, cfg.ObfsChar, tc.mode == modeObfs)
					}
				}
				if got, matched := run(); got != want || matched != wantMatched {
					b.Fatalf("strategy bytes = % x, %t; want % x, %t", got, matched, want, wantMatched)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					benchmarkStringSink, benchmarkBoolSink = run()
				}
			})
		}
	}
}

func benchmarkFoldedRewriteInput(textBytes, scalars int, script, match string) (string, string) {
	patternPrefix, sourcePrefix := "A", "a"
	patternSuffix, sourceSuffix := "B", "b"
	switch script {
	case "sigma":
		patternPrefix, sourcePrefix = "Σ", "σ"
	case "kelvin":
		patternPrefix, sourcePrefix = "K", "K"
	case "invalid-utf8":
		patternPrefix = string([]byte{0xff})
		sourcePrefix = string([]byte{0xfe})
	}
	term := strings.Repeat(patternPrefix, scalars-1) + patternSuffix
	sourceTerm := strings.Repeat(sourcePrefix, scalars-1) + sourceSuffix
	if match == "overlap" {
		term = strings.Repeat(patternPrefix, scalars)
		sourceTerm = strings.Repeat(sourcePrefix, scalars)
	}
	repeatBytes := func(unit string, size int) string {
		return strings.Repeat(unit, size/len(unit)) + strings.Repeat("q", size%len(unit))
	}
	switch match {
	case "none":
		return repeatBytes(sourcePrefix, textBytes), term
	case "sparse":
		return repeatBytes(sourcePrefix, textBytes-len(sourceTerm)) + sourceTerm, term
	case "dense":
		return repeatBytes(sourceTerm, textBytes), term
	case "overlap":
		return repeatBytes(sourcePrefix, textBytes), term
	default:
		panic("unknown folded rewrite benchmark match")
	}
}

func benchmarkStrategyName(impl string, mode mode, rules, text int, pattern, match, set string) string {
	return fmt.Sprintf("impl=%s/mode=%s/rules=%d/text=%d/pattern=%s/match=%s/set=%s", impl, mode, rules, text, pattern, match, set)
}

func benchmarkBaselineName(mode mode, rules, text int, pattern, match, set string) string {
	return benchmarkStrategyName("baseline", mode, rules, text, pattern, match, set)
}

func benchmarkRules(count int, target string) []compiledRule {
	rules := make([]compiledRule, count)
	for i := range rules {
		term := fmt.Sprintf("term-%03d", i)
		rules[i] = compiledRule{Term: term}
	}
	if target != "" {
		rules[len(rules)-1] = compiledRule{Term: target}
	}
	return rules
}

func benchmarkFoldTransitionRules(count int) []compiledRule {
	rules := make([]compiledRule, count)
	for i := range rules {
		term := strings.Repeat("a", 4) + fmt.Sprintf("-%03d", i)
		rules[i] = compiledRule{Term: term}
	}
	return rules
}

func benchmarkSnapshot(mode mode, ignoreCase bool, rules []compiledRule) *configSnapshot {
	cfg := &configSnapshot{
		Mode:       mode,
		IgnoreCase: ignoreCase,
		Rules:      rules,
		Formats:    scopeSet{"openai": {}},
		Roles:      scopeSet{"user": {}},
		ObfsChar:   "​",
	}
	if err := compileSnapshot(cfg); err != nil {
		panic(err)
	}
	return cfg
}

func benchmarkPositionedText(size int, match, position string) string {
	if len(match) > size {
		panic("benchmark match exceeds text size")
	}
	padding := size - len(match)
	prefix := padding
	switch position {
	case "first":
		prefix = 0
	case "middle":
		prefix = padding / 2
	case "last":
	default:
		panic("unknown benchmark match position")
	}
	return strings.Repeat("x", prefix) + match + strings.Repeat("x", padding-prefix)
}

func benchmarkSizedText(size int, match string) string {
	if len(match) > size {
		panic("benchmark match exceeds text size")
	}
	return strings.Repeat("x", size-len(match)) + match
}
