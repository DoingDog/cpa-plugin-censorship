package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func BenchmarkTransformMatrix(b *testing.B) {
	bodySizes := []int{1 << 10, 1 << 20, 20 << 20}
	wordCounts := []int{0, 1, 32, 256, 1024}
	for _, size := range bodySizes {
		for _, words := range wordCounts {
			for _, fold := range []bool{false, true} {
				name := fmt.Sprintf("body=%d/words=%d/fold=%t", size, words, fold)
				b.Run(name, func(b *testing.B) {
					body, cfg := benchmarkFixture(size, words, fold)
					b.ReportAllocs()
					b.SetBytes(int64(len(body)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := transformRequest(body, "openai", cfg); err != nil {
							b.Fatal(err)
						}
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
	}{
		{name: "mode=block/match=none/nodes=1", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1},
		{name: "mode=block/match=sparse/nodes=1000", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1000, lastText: "SECRET"},
		{name: "mode=strip/match=dense", yaml: "mode: strip\nwords: [SECRET]\n", text: strings.Repeat("SECRET", 128), nodes: 1},
		{name: "mode=strip/match=overlap", yaml: "mode: strip\nwords: [aa]\n", text: "aaa", nodes: 1},
		{name: "mode=strip/match=cascade", yaml: "mode: strip\nwords: [AB, x]\n", text: "ABxABx", nodes: 1},
		{name: "mode=obfs/match=sparse", yaml: "mode: obfs\nwords: [SECRET]\n", text: "SECRET", nodes: 1},
		{name: "fold=ascii", yaml: "mode: strip\nignore_case: true\nwords: [Alpha]\n", text: "aLPHA", nodes: 1},
		{name: "fold=sigma", yaml: "mode: strip\nignore_case: true\nwords: [Σ]\n", text: "ςΣσ", nodes: 1},
		{name: "fold=kelvin", yaml: "mode: strip\nignore_case: true\nwords: [K]\n", text: "K", nodes: 1},
		{name: "fold=full-fold-miss", yaml: "mode: strip\nignore_case: true\nwords: [straße]\n", text: "STRASSE", nodes: 1},
		{name: "excluded=20MiB/before", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1, excludedPosition: "before"},
		{name: "excluded=20MiB/middle", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 2, excludedPosition: "middle"},
		{name: "excluded=20MiB/after", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1, excludedPosition: "after"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			cfg := mustBenchmarkConfig(b, tc.yaml)
			body := benchmarkScenarioBody(tc.text, tc.lastText, tc.nodes, tc.excludedPosition)
			if tc.excludedPosition != "" {
				got, err := transformRequest(body, "openai", cfg)
				if err != nil || got.Invalid || got.Blocked != nil || len(got.Body) != 0 {
					b.Fatalf("excluded payload preflight = %#v, %v", got, err)
				}
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := transformRequest(body, "openai", cfg); err != nil {
					b.Fatal(err)
				}
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
		for pb.Next() {
			if _, err := transformRequest(body, "openai", loadedSnapshot()); err != nil {
				b.Error(err)
			}
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
		b.ReportAllocs()
		b.SetBytes(int64(len(request)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := handleMethod(pluginabi.MethodRequestInterceptBefore, request); err != nil {
				b.Fatal(err)
			}
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
		messages = append(messages, `{"role":"user","content":`+strconv.Quote(nodeText)+`}`)
	}
	if excludedPosition != "" {
		payload := strings.Repeat("SECRET", (20<<20)/len("SECRET")+1)[:20<<20]
		image := `{"role":"user","content":[{"type":"image_url","image_url":{"url":` + strconv.Quote(payload) + `}}]}`
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

func BenchmarkRebuildChangedSpans(b *testing.B) {
	runBenchmarkRebuildChangedSpans(b)
}

func BenchmarkSuccessEnvelope(b *testing.B) {
	runBenchmarkSuccessEnvelope(b)
}

func BenchmarkExactRewriteStrategies(b *testing.B) {
	runBenchmarkExactRewriteStrategies(b)
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

var (
	benchmarkBoolSink      bool
	benchmarkBytesSink     []byte
	benchmarkEnvelopeSink  []byte
	benchmarkErrorSink     error
	benchmarkFoldRuleSink  int
	benchmarkSpansSink     []textSpan
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
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":` + strconv.Quote(text) + `},{"text":"ignored","functionCall":{"name":"noop"}},{"text":"tail"}]},{"role":"model","parts":[{"text":"assistant"}]}]}`)
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
	body := []byte(`{"messages":[{"role":"user","content":` + strconv.Quote(first) + `},{"role":"user","content":"BLOCKME"}]}`)
	spans, err := selectTextSpans(body, "openai", scopeSet{"user": {}})
	if err != nil || len(spans) != 2 {
		b.Fatalf("selectTextSpans() = %#v, %v; want two user spans", spans, err)
	}
	spans[0].Text = strings.TrimSuffix(first, "BLOCKME")
	spans[0].Changed = true
	spans[1].Text = ""
	spans[1].Changed = true
	want := []byte(`{"messages":[{"role":"user","content":` + strconv.Quote(spans[0].Text) + `},{"role":"user","content":""}]}`)
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
	cases := []struct {
		rules, text int
		set         string
	}{
		{rules: 8, text: 4 << 10, set: "calibration"},
		{rules: 32, text: 16 << 10, set: "calibration"},
		{rules: 128, text: 64 << 10, set: "holdout"},
	}
	const term = "BLOCKME"
	for _, tc := range cases {
		tc := tc
		b.Run(benchmarkBaselineName(modeStrip, tc.rules, tc.text, "literal", "last", tc.set), func(b *testing.B) {
			rules := benchmarkRules(tc.rules, term)
			cfg := benchmarkSnapshot(modeStrip, false, rules)
			text := benchmarkSizedText(tc.text, term)
			body := benchmarkScenarioBody(text, "", 1, "")
			want := benchmarkScenarioBody(strings.TrimSuffix(text, term), "", 1, "")
			got, err := transformRequest(body, "openai", cfg)
			if err != nil || got.Invalid || got.Blocked != nil || !bytes.Equal(got.Body, want) {
				b.Fatalf("transformRequest() = %#v, %v; want stripped body", got, err)
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
	}{
		{rules: 8, text: 4 << 10, set: "calibration"},
		{rules: 32, text: 16 << 10, set: "calibration"},
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
		cfg := benchmarkSnapshot(modeStrip, true, benchmarkRules(tc.rules, ""))
		text := benchmarkSizedText(tc.text, "")
		for _, strategy := range strategies {
			strategy := strategy
			b.Run(benchmarkStrategyName(strategy.name, modeStrip, tc.rules, tc.text, "folded", "none", tc.set), func(b *testing.B) {
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

func benchmarkFoldRewriteStrategy(text string, cfg *configSnapshot, preflight bool) bool {
	if preflight {
		spans := [...]textSpan{{Text: text}}
		_, changed := applyMode(spans[:], cfg)
		return changed
	}
	changed := false
	for _, rule := range cfg.Rules {
		var matched bool
		text, matched = stripRule(text, rule, true)
		changed = changed || matched
	}
	return changed
}

func runBenchmarkExactBlockStrategies(b *testing.B) {
	cases := []struct {
		rules, text int
		set         string
	}{
		{rules: 32, text: 4 << 10, set: "calibration"},
		{rules: 64, text: 16 << 10, set: "calibration"},
		{rules: 128, text: 64 << 10, set: "calibration"},
		{rules: 256, text: 16 << 10, set: "holdout"},
	}
	const term = "BLOCKME"
	for _, tc := range cases {
		tc := tc
		b.Run(benchmarkBaselineName(modeBlock, tc.rules, tc.text, "literal", "last", tc.set), func(b *testing.B) {
			cfg := benchmarkSnapshot(modeBlock, false, benchmarkRules(tc.rules, term))
			body := benchmarkScenarioBody(benchmarkSizedText(tc.text, term), "", 1, "")
			got, err := transformRequest(body, "openai", cfg)
			if err != nil || got.Invalid || got.Body != nil || got.Blocked == nil || got.Blocked.Term != term || got.Blocked.Role != "user" {
				b.Fatalf("transformRequest() = %#v, %v; want block for %q", got, err, term)
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

func runBenchmarkFoldedRewriteStrategies(b *testing.B) {
	cases := []struct {
		text, scalars int
		set           string
	}{
		{text: 4 << 10, scalars: 4, set: "calibration"},
		{text: 16 << 10, scalars: 8, set: "calibration"},
		{text: 64 << 10, scalars: 16, set: "holdout"},
	}
	for _, tc := range cases {
		tc := tc
		b.Run(benchmarkBaselineName(modeStrip, 1, tc.text, fmt.Sprintf("fold-%dscalars", tc.scalars), "folded", tc.set), func(b *testing.B) {
			term := strings.Repeat("A", tc.scalars)
			matchedText := strings.ToLower(term)
			cfg := benchmarkSnapshot(modeStrip, true, benchmarkRules(1, term))
			text := benchmarkSizedText(tc.text, matchedText)
			body := benchmarkScenarioBody(text, "", 1, "")
			want := benchmarkScenarioBody(strings.TrimSuffix(text, matchedText), "", 1, "")
			got, err := transformRequest(body, "openai", cfg)
			if err != nil || got.Invalid || got.Blocked != nil || !bytes.Equal(got.Body, want) {
				b.Fatalf("transformRequest() = %#v, %v; want folded stripped body", got, err)
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

func benchmarkSizedText(size int, match string) string {
	if len(match) > size {
		panic("benchmark match exceeds text size")
	}
	return strings.Repeat("x", size-len(match)) + match
}
