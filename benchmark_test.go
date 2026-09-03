package main

import (
	"encoding/json"
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
		rules[i] = compiledRule{Term: term, Runes: []rune(term)}
	}
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: fold,
		Rules:      rules,
		Formats:    scopeSet{"openai": {}},
		Roles:      scopeSet{"user": {}},
		ObfsChar:   "​",
	}
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
