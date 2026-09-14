package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type filterMode string

const (
	filterModeExclude filterMode = "exclude"
	filterModeInclude filterMode = "include"
)

type filterLogic string

const (
	filterLogicOr  filterLogic = "or"
	filterLogicAnd filterLogic = "and"
)

type compiledFilterPattern struct {
	Text        string
	Runes       []rune
	CallerScope string
}

type requestFilter struct {
	Mode    filterMode
	Logic   filterLogic
	APIKeys []compiledFilterPattern
	Models  []compiledFilterPattern
}

func (f requestFilter) enabled() bool {
	return len(f.APIKeys) != 0 || len(f.Models) != 0
}

func compileRequestFilter(filter *requestFilter) {
	for i := range filter.APIKeys {
		pattern := &filter.APIKeys[i]
		pattern.Runes = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Runes = []rune(pattern.Text)
		} else if pattern.Text == strings.TrimSpace(pattern.Text) {
			pattern.CallerScope = callerScope(pattern.Text)
		}
	}
	for i := range filter.Models {
		pattern := &filter.Models[i]
		pattern.Runes = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Runes = []rune(pattern.Text)
		}
	}
}

const (
	callerScopeMetadataKey = "caller_scope"
	callerScopeDomain      = "cli-proxy-api:caller-scope:v1\x00"
)

func callerScope(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(callerScopeDomain + value))
	return hex.EncodeToString(sum[:])
}

func callerScopeFromMetadata(metadata map[string]any) string {
	scope, _ := metadata[callerScopeMetadataKey].(string)
	if len(scope) != sha256.Size*2 {
		return ""
	}
	for i := range scope {
		if !('0' <= scope[i] && scope[i] <= '9') && !('a' <= scope[i] && scope[i] <= 'f') {
			return ""
		}
	}
	return scope
}

func matchFilterGlob(pattern, value []rune) bool {
	patternIndex, valueIndex := 0, 0
	starIndex, retryValueIndex := -1, 0
	for valueIndex < len(value) {
		switch {
		case patternIndex < len(pattern) && (pattern[patternIndex] == '?' || pattern[patternIndex] == value[valueIndex]):
			patternIndex++
			valueIndex++
		case patternIndex < len(pattern) && pattern[patternIndex] == '*':
			starIndex = patternIndex
			patternIndex++
			retryValueIndex = valueIndex
		case starIndex >= 0:
			patternIndex = starIndex + 1
			retryValueIndex++
			valueIndex = retryValueIndex
		default:
			return false
		}
	}
	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

func scopeBoundHeaderValue(headers http.Header, name, scope string, bearer bool) string {
	for key, values := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			if bearer {
				parts := strings.SplitN(value, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
					value = parts[1]
				}
			}
			candidate := strings.TrimSpace(value)
			if candidate != "" && callerScope(candidate) == scope {
				return candidate
			}
		}
	}
	return ""
}

func authenticatedCallerAPIKey(headers http.Header, scope string) string {
	if scope == "" {
		return ""
	}
	if candidate := scopeBoundHeaderValue(headers, "Authorization", scope, true); candidate != "" {
		return candidate
	}
	if candidate := scopeBoundHeaderValue(headers, "X-Goog-Api-Key", scope, false); candidate != "" {
		return candidate
	}
	return scopeBoundHeaderValue(headers, "X-Api-Key", scope, false)
}

func (f requestFilter) matchesAPIKey(headers http.Header, metadata map[string]any) bool {
	scope := callerScopeFromMetadata(metadata)
	if scope == "" {
		return false
	}
	hasWildcard := false
	for i := range f.APIKeys {
		pattern := &f.APIKeys[i]
		if pattern.Runes != nil {
			hasWildcard = true
			continue
		}
		if pattern.CallerScope != "" && pattern.CallerScope == scope {
			return true
		}
	}
	if !hasWildcard {
		return false
	}
	candidate := authenticatedCallerAPIKey(headers, scope)
	if candidate == "" {
		return false
	}
	valueRunes := []rune(candidate)
	for i := range f.APIKeys {
		pattern := &f.APIKeys[i]
		if pattern.Runes != nil && matchFilterGlob(pattern.Runes, valueRunes) {
			return true
		}
	}
	return false
}

func (f requestFilter) matchesModel(value string) bool {
	if value == "" {
		return false
	}
	var valueRunes []rune
	for i := range f.Models {
		pattern := &f.Models[i]
		if pattern.Runes == nil {
			if pattern.Text == value {
				return true
			}
			continue
		}
		if valueRunes == nil {
			valueRunes = []rune(value)
		}
		if matchFilterGlob(pattern.Runes, valueRunes) {
			return true
		}
	}
	return false
}

func (f requestFilter) shouldProcess(request *pluginapi.RequestInterceptRequest) bool {
	if !f.enabled() {
		return true
	}
	apiConfigured := len(f.APIKeys) != 0
	modelConfigured := len(f.Models) != 0
	var matched bool
	switch {
	case apiConfigured && modelConfigured && f.Logic == filterLogicAnd:
		matched = f.matchesModel(request.RequestedModel) && f.matchesAPIKey(request.Headers, request.Metadata)
	case apiConfigured && modelConfigured:
		matched = f.matchesModel(request.RequestedModel) || f.matchesAPIKey(request.Headers, request.Metadata)
	case apiConfigured:
		matched = f.matchesAPIKey(request.Headers, request.Metadata)
	default:
		matched = f.matchesModel(request.RequestedModel)
	}
	return matched == (f.Mode == filterModeInclude)
}
