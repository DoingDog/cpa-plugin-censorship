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

const (
	modelExecutionSourceKey      = "source"
	modelExecutionCallbackSource = "plugin_host_model_callback"
)

func (f requestFilter) modelSubject(request *pluginapi.RequestInterceptRequest) string {
	if source, _ := request.Metadata[modelExecutionSourceKey].(string); source == modelExecutionCallbackSource {
		return request.Model
	}
	return request.RequestedModel
}

type compiledFilterPattern struct {
	Text        string
	Glob        *compiledFilterGlob
	CallerScope string
}

type compiledFilterGlob struct {
	finalState int
	stars      []uint64
	questions  []uint64
	literals   map[rune]globLiteralPositions
}

type globLiteralPositions struct {
	sparse []int
	dense  []uint64
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
		pattern.Glob = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Glob = compileFilterGlob([]rune(pattern.Text))
		} else if pattern.Text == strings.TrimSpace(pattern.Text) {
			pattern.CallerScope = callerScope(pattern.Text)
		}
	}
	for i := range filter.Models {
		pattern := &filter.Models[i]
		pattern.Glob = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Glob = compileFilterGlob([]rune(pattern.Text))
		}
	}
}

func compileFilterGlob(pattern []rune) *compiledFilterGlob {
	tokens := make([]rune, 0, len(pattern))
	for _, token := range pattern {
		if token == '*' && len(tokens) != 0 && tokens[len(tokens)-1] == '*' {
			continue
		}
		tokens = append(tokens, token)
	}

	words := (len(tokens) + 64) / 64
	matcher := &compiledFilterGlob{
		finalState: len(tokens),
		stars:      make([]uint64, words),
		questions:  make([]uint64, words),
	}
	for state, token := range tokens {
		switch token {
		case '*':
			matcher.stars[state/64] |= uint64(1) << (state % 64)
		case '?':
			matcher.questions[state/64] |= uint64(1) << (state % 64)
		default:
			literal := matcher.literals[token]
			literal.sparse = append(literal.sparse, state)
			if matcher.literals == nil {
				matcher.literals = make(map[rune]globLiteralPositions)
			}
			matcher.literals[token] = literal
		}
	}
	for token, literal := range matcher.literals {
		if len(literal.sparse) < words {
			continue
		}
		literal.dense = make([]uint64, words)
		for _, state := range literal.sparse {
			literal.dense[state/64] |= uint64(1) << (state % 64)
		}
		literal.sparse = nil
		matcher.literals[token] = literal
	}
	return matcher
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

func (matcher *compiledFilterGlob) matches(value []rune) bool {
	active := make([]uint64, len(matcher.stars))
	next := make([]uint64, len(matcher.stars))
	active[0] = 1
	matcher.expandStars(active)

	for _, token := range value {
		literal, hasLiteral := matcher.literals[token]
		carry := uint64(0)
		for word := range active {
			stepped := active[word] & matcher.questions[word]
			if hasLiteral && literal.dense != nil {
				stepped |= active[word] & literal.dense[word]
			}
			next[word] = (active[word] & matcher.stars[word]) | (stepped << 1) | carry
			carry = stepped >> 63
		}
		if hasLiteral {
			for _, state := range literal.sparse {
				if active[state/64]&(uint64(1)<<(state%64)) != 0 {
					next[(state+1)/64] |= uint64(1) << ((state + 1) % 64)
				}
			}
		}
		matcher.expandStars(next)
		active, next = next, active
	}
	return active[matcher.finalState/64]&(uint64(1)<<(matcher.finalState%64)) != 0
}

func (matcher *compiledFilterGlob) expandStars(states []uint64) {
	carry := uint64(0)
	for word := range states {
		starred := states[word] & matcher.stars[word]
		states[word] |= (starred << 1) | carry
		carry = starred >> 63
	}
}

func matchFilterGlob(pattern, value []rune) bool {
	return compileFilterGlob(pattern).matches(value)
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
		if pattern.Glob != nil {
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
		if pattern.Glob != nil && pattern.Glob.matches(valueRunes) {
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
		if pattern.Glob == nil {
			if pattern.Text == value {
				return true
			}
			continue
		}
		if valueRunes == nil {
			valueRunes = []rune(value)
		}
		if pattern.Glob.matches(valueRunes) {
			return true
		}
	}
	return false
}

func (f requestFilter) shouldProcess(request *pluginapi.RequestInterceptRequest) bool {
	if !f.enabled() {
		return true
	}
	model := f.modelSubject(request)
	apiConfigured := len(f.APIKeys) != 0
	modelConfigured := len(f.Models) != 0
	var matched bool
	switch {
	case apiConfigured && modelConfigured && f.Logic == filterLogicAnd:
		matched = f.matchesModel(model) && f.matchesAPIKey(request.Headers, request.Metadata)
	case apiConfigured && modelConfigured:
		matched = f.matchesModel(model) || f.matchesAPIKey(request.Headers, request.Metadata)
	case apiConfigured:
		matched = f.matchesAPIKey(request.Headers, request.Metadata)
	default:
		matched = f.matchesModel(model)
	}
	return matched == (f.Mode == filterModeInclude)
}
