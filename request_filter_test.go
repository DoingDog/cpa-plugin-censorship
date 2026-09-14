package main

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	testKeyCallerScope = "a420e246227b259b532446574f6fd7719cc2d7f551efd9944f93422b96811a56"
	accountCallerScope = "70d7f532bbb4b34d73d8b94cd09d49cb1836a9a1d81949e0fd1c5d3b19d0dc37"
)

func TestMatchFilterGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{name: "exact", pattern: "abc", value: "abc", want: true},
		{name: "exact mismatch", pattern: "abc", value: "ab", want: false},
		{name: "prefix star", pattern: "ab*", value: "ab/甲", want: true},
		{name: "suffix star", pattern: "*bc", value: "a/bc", want: true},
		{name: "middle star empty", pattern: "a*c", value: "ac", want: true},
		{name: "middle star", pattern: "a*c", value: "a/甲/c", want: true},
		{name: "question Unicode scalar", pattern: "a?c", value: "a甲c", want: true},
		{name: "question one only", pattern: "a?c", value: "a甲乙c", want: false},
		{name: "anchored prefix", pattern: "ab*", value: "zab", want: false},
		{name: "anchored suffix", pattern: "*bc", value: "bcz", want: false},
		{name: "case sensitive", pattern: "Key-*", value: "key-a", want: false},
		{name: "brackets literal", pattern: "m[0]", value: "m[0]", want: true},
		{name: "brackets not class", pattern: "m[0]", value: "m0", want: false},
		{name: "only star", pattern: "*", value: "anything", want: true},
		{name: "star empty", pattern: "*", value: "", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := matchFilterGlob([]rune(test.pattern), []rune(test.value)); got != test.want {
				t.Fatalf("matchFilterGlob(%q, %q) = %t, want %t", test.pattern, test.value, got, test.want)
			}
		})
	}
}

func TestCallerScope(t *testing.T) {
	if got, want := callerScope("test-key"), testKeyCallerScope; got != want {
		t.Fatalf("callerScope() = %q, want %q", got, want)
	}
	if callerScope(" \t\n") != "" {
		t.Fatal("blank caller scope was non-empty")
	}
}

func TestRequestFilterMatchesAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		headers  http.Header
		metadata map[string]any
		want     bool
	}{
		{
			name:     "exact matching metadata scope without headers",
			patterns: []string{"test-key"},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "exact missing scope",
			patterns: []string{"test-key"},
			want:     false,
		},
		{
			name:     "exact uppercase scope",
			patterns: []string{"test-key"},
			metadata: map[string]any{callerScopeMetadataKey: strings.ToUpper(testKeyCallerScope)},
			want:     false,
		},
		{
			name:     "exact non hexadecimal scope",
			patterns: []string{"test-key"},
			metadata: map[string]any{callerScopeMetadataKey: strings.Repeat("g", 64)},
			want:     false,
		},
		{
			name:     "exact wrong scope",
			patterns: []string{"test-key"},
			metadata: map[string]any{callerScopeMetadataKey: accountCallerScope},
			want:     false,
		},
		{
			name:     "whitespace exact never matches",
			patterns: []string{" test-key "},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     false,
		},
		{
			name:     "wildcard Authorization bearer",
			patterns: []string{"test-*"},
			headers:  http.Header{"Authorization": {"Bearer test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard lower case bearer",
			patterns: []string{"test-?ey"},
			headers:  http.Header{"Authorization": {"bearer test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard skips forged Authorization value",
			patterns: []string{"test-*"},
			headers:  http.Header{"Authorization": {"Bearer forged", "Bearer test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard noncanonical authorization key",
			patterns: []string{"test-*"},
			headers:  http.Header{"authorization": {"Bearer test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard mixed case Google API key",
			patterns: []string{"test-*"},
			headers:  http.Header{"x-GoOg-aPi-KeY": {"test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard mixed case API key",
			patterns: []string{"test-*"},
			headers:  http.Header{"X-aPi-kEy": {"test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard canonical Google API key",
			patterns: []string{"test-*"},
			headers:  http.Header{"X-Goog-Api-Key": {"test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard canonical API key",
			patterns: []string{"test-*"},
			headers:  http.Header{"X-Api-Key": {"test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "wildcard rejects Cookie credential",
			patterns: []string{"test-*"},
			headers:  http.Header{"Cookie": {"credential=test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     false,
		},
		{
			name:     "wildcard rejects Proxy Authorization credential",
			patterns: []string{"test-*"},
			headers:  http.Header{"Proxy-Authorization": {"Bearer test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     false,
		},
		{
			name:     "wildcard rejects credential for another scope",
			patterns: []string{"test-*"},
			headers:  http.Header{"X-Goog-Api-Key": {"test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: accountCallerScope},
			want:     false,
		},
		{
			name:     "wildcard requires scope",
			patterns: []string{"test-*"},
			headers:  http.Header{"Authorization": {"Bearer test-key"}},
			want:     false,
		},
		{
			name:     "query only exact key matches scope",
			patterns: []string{"test-key"},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
		{
			name:     "principal mismatch without headers",
			patterns: []string{"test-key"},
			metadata: map[string]any{callerScopeMetadataKey: accountCallerScope},
			want:     false,
		},
		{
			name:     "wildcard patterns use OR",
			patterns: []string{"never-*", "test-*"},
			headers:  http.Header{"X-Api-Key": {"test-key"}},
			metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
			want:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter := requestFilter{APIKeys: make([]compiledFilterPattern, len(test.patterns))}
			for i, pattern := range test.patterns {
				filter.APIKeys[i].Text = pattern
			}
			compileRequestFilter(&filter)
			if got := filter.matchesAPIKey(test.headers, test.metadata); got != test.want {
				t.Fatalf("matchesAPIKey() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRequestFilterShouldProcess(t *testing.T) {
	tests := []struct {
		name            string
		mode            filterMode
		logic           filterLogic
		apiConfigured   bool
		apiMatches      bool
		modelConfigured bool
		modelMatches    bool
		want            bool
	}{
		{name: "include or no dimensions", mode: filterModeInclude, logic: filterLogicOr, want: true},
		{name: "exclude and no dimensions", mode: filterModeExclude, logic: filterLogicAnd, want: true},
		{name: "include or matching API key", mode: filterModeInclude, logic: filterLogicOr, apiConfigured: true, apiMatches: true, want: true},
		{name: "include and nonmatching API key", mode: filterModeInclude, logic: filterLogicAnd, apiConfigured: true, want: false},
		{name: "exclude or matching API key", mode: filterModeExclude, logic: filterLogicOr, apiConfigured: true, apiMatches: true, want: false},
		{name: "exclude and nonmatching API key", mode: filterModeExclude, logic: filterLogicAnd, apiConfigured: true, want: true},
		{name: "include or matching model", mode: filterModeInclude, logic: filterLogicOr, apiConfigured: true, modelConfigured: true, modelMatches: true, want: true},
		{name: "include or no matching dimensions", mode: filterModeInclude, logic: filterLogicOr, apiConfigured: true, modelConfigured: true, want: false},
		{name: "include and matching dimensions", mode: filterModeInclude, logic: filterLogicAnd, apiConfigured: true, apiMatches: true, modelConfigured: true, modelMatches: true, want: true},
		{name: "include and nonmatching model", mode: filterModeInclude, logic: filterLogicAnd, apiConfigured: true, apiMatches: true, modelConfigured: true, want: false},
		{name: "exclude or matching API key", mode: filterModeExclude, logic: filterLogicOr, apiConfigured: true, apiMatches: true, modelConfigured: true, want: false},
		{name: "exclude or no matching dimensions", mode: filterModeExclude, logic: filterLogicOr, apiConfigured: true, modelConfigured: true, want: true},
		{name: "exclude and nonmatching model", mode: filterModeExclude, logic: filterLogicAnd, apiConfigured: true, apiMatches: true, modelConfigured: true, want: true},
		{name: "exclude and matching dimensions", mode: filterModeExclude, logic: filterLogicAnd, apiConfigured: true, apiMatches: true, modelConfigured: true, modelMatches: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter := requestFilter{Mode: test.mode, Logic: test.logic}
			if test.apiConfigured {
				filter.APIKeys = []compiledFilterPattern{{Text: "test-key", CallerScope: testKeyCallerScope}}
			}
			if test.modelConfigured {
				filter.Models = []compiledFilterPattern{{Text: "target"}}
			}
			request := &pluginapi.RequestInterceptRequest{RequestedModel: "other"}
			if test.apiMatches {
				request.Metadata = map[string]any{callerScopeMetadataKey: testKeyCallerScope}
			} else if test.apiConfigured {
				request.Metadata = map[string]any{callerScopeMetadataKey: accountCallerScope}
			}
			if test.modelMatches {
				request.RequestedModel = "target"
			}
			if got := filter.shouldProcess(request); got != test.want {
				t.Fatalf("shouldProcess() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRequestFilterShouldProcessUsesRequestedModel(t *testing.T) {
	filter := requestFilter{
		Mode:   filterModeInclude,
		Logic:  filterLogicOr,
		Models: []compiledFilterPattern{{Text: "target"}},
	}
	request := &pluginapi.RequestInterceptRequest{
		Model:          "target",
		RequestedModel: "other",
		Body:           []byte(`{"model":"target"}`),
	}
	if filter.shouldProcess(request) {
		t.Fatal("shouldProcess() matched Model or body instead of RequestedModel")
	}
}

func TestRequestFilterShouldProcessRejectsMissingRequestedModel(t *testing.T) {
	filter := requestFilter{
		Mode:   filterModeInclude,
		Logic:  filterLogicOr,
		Models: []compiledFilterPattern{{Text: "*", Runes: []rune("*")}},
	}
	request := &pluginapi.RequestInterceptRequest{
		Model: "anything",
		Body:  []byte(`{"model":"anything"}`),
	}
	if filter.shouldProcess(request) {
		t.Fatal("shouldProcess() matched missing RequestedModel")
	}
}

func TestRequestFilterShouldProcessRejectsWhitespaceExactAPIKey(t *testing.T) {
	request := &pluginapi.RequestInterceptRequest{
		Metadata: map[string]any{callerScopeMetadataKey: testKeyCallerScope},
	}
	for _, test := range []struct {
		mode filterMode
		want bool
	}{
		{mode: filterModeInclude, want: false},
		{mode: filterModeExclude, want: true},
	} {
		filter := requestFilter{
			Mode:    test.mode,
			Logic:   filterLogicOr,
			APIKeys: []compiledFilterPattern{{Text: " test-key "}},
		}
		compileRequestFilter(&filter)
		if got := filter.shouldProcess(request); got != test.want {
			t.Fatalf("shouldProcess() with %q = %t, want %t", test.mode, got, test.want)
		}
	}
}

func TestRequestFilterGatesBeforeBodyValidation(t *testing.T) {
	cases := []struct {
		name       string
		configYAML string
		request    pluginapi.RequestInterceptRequest
		bypass     bool
	}{
		{
			name:       "include skips nonmatching model malformed body",
			configYAML: "filter_mode: include\nfilter:\n  models: [target-*]\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "include-other",
				SourceFormat:   "openai",
				RequestedModel: "other",
				Body:           []byte(`not-json`),
			},
			bypass: true,
		},
		{
			name:       "include processes matching model malformed body",
			configYAML: "filter_mode: include\nfilter:\n  models: [target-*]\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "include-target",
				SourceFormat:   "openai",
				RequestedModel: "target-model",
				Body:           []byte(`not-json`),
			},
		},
		{
			name:       "exclude skips matching model duplicate members",
			configYAML: "filter_mode: exclude\nfilter:\n  models: [target-*]\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "exclude-target",
				SourceFormat:   "openai",
				RequestedModel: "target-model",
				Body:           []byte(`{"messages":[],"messages":[]}`),
			},
			bypass: true,
		},
		{
			name:       "exclude processes nonmatching model duplicate members",
			configYAML: "filter_mode: exclude\nfilter:\n  models: [target-*]\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "exclude-other",
				SourceFormat:   "openai",
				RequestedModel: "other",
				Body:           []byte(`{"messages":[],"messages":[]}`),
			},
		},
		{
			name:       "no rules skip malformed identity and body",
			configYAML: "filter_mode: include\nfilter:\n  api-keys: [test-*]\nwords: {}\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:    "no-rules",
				SourceFormat: "openai",
				Headers:      http.Header{"Authorization": {"Bearer test-key"}},
				Metadata:     map[string]any{callerScopeMetadataKey: 42},
				Body:         []byte(`not-json`),
			},
			bypass: true,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registerConfig(t, test.configYAML)
			response, err := callInterceptRequest(test.request)
			if err != nil {
				t.Fatal(err)
			}
			if test.bypass {
				if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{}) {
					t.Fatalf("response = %#v, want zero-value response", response)
				}
				return
			}
			if !response.Terminate || response.StatusCode != 400 || !strings.Contains(string(response.ResponseBody), "censorship_invalid_request") {
				t.Fatalf("response = %#v, want terminated censorship_invalid_request", response)
			}
		})
	}
}

func TestRequestFilterGatesAllSupportedFormats(t *testing.T) {
	formats := []struct {
		name   string
		format string
		body   []byte
	}{
		{name: "openai", format: "openai", body: []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"SECRET"}]}`)},
		{name: "openai-response", format: "openai-response", body: []byte(`{"model":"body-decoy","input":"SECRET"}`)},
		{name: "claude", format: "claude", body: []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"SECRET"}]}`)},
		{name: "gemini", format: "gemini", body: []byte(`{"model":"body-decoy","contents":[{"role":"user","parts":[{"text":"SECRET"}]}]}`)},
		{name: "interactions", format: "interactions", body: []byte(`{"model":"body-decoy","input":"SECRET"}`)},
	}
	registerConfig(t, "filter_mode: include\nfilter:\n  models: [target-*]\nwords:\n  block: [SECRET]\n")

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			response, err := callInterceptRequest(pluginapi.RequestInterceptRequest{
				RequestID:      "matching-" + format.name,
				SourceFormat:   format.format,
				RequestedModel: "target-model",
				Model:          "ignored-current-model",
				Body:           format.body,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !response.Terminate || response.StatusCode != 400 || !strings.Contains(string(response.ResponseBody), "censorship_blocked") {
				t.Fatalf("matching response = %#v, want terminated censorship_blocked", response)
			}

			response, err = callInterceptRequest(pluginapi.RequestInterceptRequest{
				RequestID:      "nonmatching-" + format.name,
				SourceFormat:   format.format,
				RequestedModel: "other",
				Model:          "target-model",
				Body:           format.body,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{}) {
				t.Fatalf("nonmatching response = %#v, want zero-value response", response)
			}
		})
	}
}

func TestRequestFilterAPIKeyGate(t *testing.T) {
	cases := []struct {
		name       string
		configYAML string
		request    pluginapi.RequestInterceptRequest
		blocked    bool
	}{
		{
			name:       "wildcard matching caller scope and authorization",
			configYAML: "filter_mode: include\nfilter:\n  api-keys: ['test-?ey']\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:    "wildcard-match",
				SourceFormat: "openai",
				Headers:      http.Header{"Authorization": {"Bearer test-key"}},
				Metadata:     map[string]any{callerScopeMetadataKey: testKeyCallerScope},
				Body:         []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`),
			},
			blocked: true,
		},
		{
			name:       "wildcard header without caller scope",
			configYAML: "filter_mode: include\nfilter:\n  api-keys: ['test-?ey']\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:    "wildcard-no-scope",
				SourceFormat: "openai",
				Headers:      http.Header{"Authorization": {"Bearer test-key"}},
				Body:         []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`),
			},
		},
		{
			name:       "wildcard header with forged account scope",
			configYAML: "filter_mode: include\nfilter:\n  api-keys: ['test-?ey']\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:    "wildcard-forged-scope",
				SourceFormat: "openai",
				Headers:      http.Header{"Authorization": {"Bearer test-key"}},
				Metadata:     map[string]any{callerScopeMetadataKey: accountCallerScope},
				Body:         []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`),
			},
		},
		{
			name:       "exact pattern matching caller scope without authorization",
			configYAML: "filter_mode: include\nfilter:\n  api-keys: [test-key]\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:    "exact-match",
				SourceFormat: "openai",
				Metadata:     map[string]any{callerScopeMetadataKey: testKeyCallerScope},
				Body:         []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`),
			},
			blocked: true,
		},
		{
			name:       "whitespace exact pattern does not match caller scope",
			configYAML: "filter_mode: include\nfilter:\n  api-keys: [' test-key ']\nwords:\n  block: [SECRET]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:    "whitespace-exact",
				SourceFormat: "openai",
				Metadata:     map[string]any{callerScopeMetadataKey: testKeyCallerScope},
				Body:         []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`),
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registerConfig(t, test.configYAML)
			response, err := callInterceptRequest(test.request)
			if err != nil {
				t.Fatal(err)
			}
			if test.blocked {
				if !response.Terminate || response.StatusCode != 400 || !strings.Contains(string(response.ResponseBody), "censorship_blocked") {
					t.Fatalf("response = %#v, want terminated censorship_blocked", response)
				}
				return
			}
			if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{}) {
				t.Fatalf("response = %#v, want zero-value response", response)
			}
		})
	}
}
