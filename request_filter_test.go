package main

import (
	"net/http"
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
