package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

type textSpan struct {
	RawStart           int
	RawEnd             int
	Text               string
	Role               string
	Changed            bool
	SkipFoldRewrite    bool
	RequiresNonEmpty   bool
	MaxCodePoints      int
	RequiresUnmodified bool
	UnmodifiedMessage  string
	NestedDescription  *textSpan
	NestedNonString    bool
	EncodedChildren    []textSpan
}

const (
	foldRewritePreflightMinRules     = 8
	foldRewritePreflightMinTextBytes = 4 << 10
	exactByteMatcherMinRules         = 256
	exactByteMatcherMinTextBytes     = 16 << 10
	exactByteMatcherPrefixRules      = 4
	foldKMPMinPatternScalars         = 4
	foldKMPMinTextBytes              = 4 << 10
)

func useFoldRewritePreflight(ruleCount, textBytes int) bool {
	return ruleCount >= foldRewritePreflightMinRules && textBytes >= foldRewritePreflightMinTextBytes
}

func useExactByteMatcher(ruleCount, totalTextBytes int) bool {
	return ruleCount >= exactByteMatcherMinRules && totalTextBytes >= exactByteMatcherMinTextBytes
}

func useFoldedKMP(textBytes, patternScalars int) bool {
	return textBytes >= foldKMPMinTextBytes && patternScalars >= foldKMPMinPatternScalars
}

type blockMatch struct {
	Term string
	Role string
}

type transformResult struct {
	Body           []byte
	Blocked        *blockMatch
	Invalid        bool
	InvalidMessage string
}

func transformRequest(body []byte, sourceFormat string, cfg *configSnapshot) (transformResult, error) {
	if cfg == nil || len(cfg.Rules) == 0 || !cfg.Formats.has(sourceFormat) || len(cfg.Formats) == 0 || len(cfg.Roles) == 0 {
		return transformResult{}, nil
	}
	spans, err := collectTextSpans(body, sourceFormat, cfg.Roles)
	if errors.Is(err, errInvalidRequest) {
		return transformResult{Invalid: true}, nil
	}
	if err != nil {
		return transformResult{}, err
	}
	return transformSelectedSpans(body, spans, cfg)
}

func transformSelectedSpans(body []byte, spans []textSpan, cfg *configSnapshot) (transformResult, error) {
	if err := sortAndValidateSpans(spans, len(body)); err != nil {
		return transformResult{}, err
	}
	flat := spans
	var flatStarts []int
	for _, span := range spans {
		if span.EncodedChildren != nil {
			flatStarts = make([]int, len(spans)+1)
			flat = make([]textSpan, 0, len(spans))
			break
		}
	}
	if flatStarts != nil {
		// 内外层 offset 使用不同坐标，flat 按外层 token 顺序展开后不再排序。
		for i, span := range spans {
			flatStarts[i] = len(flat)
			if span.EncodedChildren == nil {
				flat = append(flat, span)
				continue
			}
			if err := sortAndValidateSpans(span.EncodedChildren, len(span.Text)); err != nil {
				return transformResult{}, err
			}
			flat = append(flat, span.EncodedChildren...)
		}
		flatStarts[len(spans)] = len(flat)
	}
	blocked, changed := applyMode(flat, cfg)
	if blocked != nil {
		return transformResult{Blocked: blocked}, nil
	}
	if !changed {
		return transformResult{}, nil
	}
	var activated []textSpan
	nonStringNested := false
	for _, span := range flat {
		if !span.Changed || span.Text != "" {
			continue
		}
		if span.NestedDescription != nil {
			activated = append(activated, *span.NestedDescription)
		}
		nonStringNested = nonStringNested || span.NestedNonString
	}
	if len(activated) > 0 {
		sort.Slice(activated, func(i, j int) bool { return activated[i].RawStart < activated[j].RawStart })
		blocked, _ = applyMode(activated, cfg)
		if blocked != nil {
			return transformResult{Blocked: blocked}, nil
		}
	}
	if nonStringNested {
		return transformResult{Invalid: true, InvalidMessage: "censorship cannot rewrite non-string tool description"}, nil
	}
	if flatStarts != nil {
		for i := range spans {
			selected := flat[flatStarts[i]:flatStarts[i+1]]
			if spans[i].EncodedChildren == nil {
				spans[i] = selected[0]
				continue
			}
			spans[i].EncodedChildren = selected
			spans[i].Changed = false
			for _, child := range selected {
				spans[i].Changed = spans[i].Changed || child.Changed
			}
			if !spans[i].Changed {
				continue
			}
			inner, err := rebuildBody([]byte(spans[i].Text), selected)
			if err != nil {
				return transformResult{}, err
			}
			spans[i].Text = string(inner)
		}
	}
	for _, span := range activated {
		if span.Changed {
			spans = append(spans, span)
		}
	}
	if len(activated) > 0 {
		if err := sortAndValidateSpans(spans, len(body)); err != nil {
			return transformResult{}, err
		}
	}
	for _, span := range spans {
		if message := changedSpanInvalidMessage(span); message != "" {
			return transformResult{Invalid: true, InvalidMessage: message}, nil
		}
		for _, child := range span.EncodedChildren {
			if message := changedSpanInvalidMessage(child); message != "" {
				return transformResult{Invalid: true, InvalidMessage: message}, nil
			}
		}
	}
	out, err := rebuildBody(body, spans)
	return transformResult{Body: out}, err
}

func changedSpanInvalidMessage(span textSpan) string {
	if !span.Changed {
		return ""
	}
	if span.RequiresUnmodified {
		if span.UnmodifiedMessage != "" {
			return span.UnmodifiedMessage
		}
		return "censorship cannot rewrite signature-bound text"
	}
	if span.RequiresNonEmpty && span.Text == "" || span.MaxCodePoints > 0 && utf8.RuneCountInString(span.Text) > span.MaxCodePoints {
		return "censorship rewrite would make a text field invalid"
	}
	return ""
}

func rulePhaseEnds(cfg *configSnapshot) (int, int) {
	if !cfg.rangesSet {
		switch cfg.Mode {
		case modeBlock:
			return len(cfg.Rules), len(cfg.Rules)
		case modeStrip:
			return 0, len(cfg.Rules)
		case modeObfs:
			return 0, 0
		default:
			return 0, 0
		}
	}

	blockEnd := cfg.BlockEnd
	if blockEnd < 0 {
		blockEnd = 0
	} else if blockEnd > len(cfg.Rules) {
		blockEnd = len(cfg.Rules)
	}
	stripEnd := cfg.StripEnd
	if stripEnd < blockEnd {
		stripEnd = blockEnd
	} else if stripEnd > len(cfg.Rules) {
		stripEnd = len(cfg.Rules)
	}
	return blockEnd, stripEnd
}

func matchExactBlock(spans []textSpan, cfg *configSnapshot) (int, string, bool) {
	blockEnd, _ := rulePhaseEnds(cfg)
	matcher := cfg.ExactBlockMatcher
	prefixCount := 0
	if matcher != nil {
		prefixCount = exactByteMatcherPrefixRules
		if prefixCount > blockEnd {
			prefixCount = blockEnd
		}
		if len(spans) == 1 {
			span := spans[0]
			for ruleIndex := 0; ruleIndex < prefixCount; ruleIndex++ {
				if strings.Contains(span.Text, cfg.Rules[ruleIndex].Term) {
					return ruleIndex, span.Role, true
				}
			}
		} else {
			for ruleIndex := 0; ruleIndex < prefixCount; ruleIndex++ {
				for _, span := range spans {
					if strings.Contains(span.Text, cfg.Rules[ruleIndex].Term) {
						return ruleIndex, span.Role, true
					}
				}
			}
		}

		totalTextBytes := 0
		if len(spans) == 1 {
			totalTextBytes = len(spans[0].Text)
		} else {
			for _, span := range spans {
				totalTextBytes += len(span.Text)
			}
		}
		if useExactByteMatcher(blockEnd, totalTextBytes) {
			if len(spans) == 1 {
				ruleIndex, matched := matcher.match(spans[0].Text)
				if matched && ruleIndex >= 0 && ruleIndex < blockEnd {
					return ruleIndex, spans[0].Role, true
				}
				return -1, "", false
			}

			bestRule := -1
			bestRole := ""
			for _, span := range spans {
				ruleIndex, matched := matcher.match(span.Text)
				if matched && ruleIndex >= 0 && ruleIndex < blockEnd && (bestRule < 0 || ruleIndex < bestRule) {
					bestRule = ruleIndex
					bestRole = span.Role
					if bestRule == prefixCount {
						break
					}
				}
			}
			return bestRule, bestRole, bestRule >= 0
		}
	}

	if len(spans) == 1 {
		span := spans[0]
		for ruleIndex := prefixCount; ruleIndex < blockEnd; ruleIndex++ {
			if strings.Contains(span.Text, cfg.Rules[ruleIndex].Term) {
				return ruleIndex, span.Role, true
			}
		}
		return -1, "", false
	}

	for ruleIndex := prefixCount; ruleIndex < blockEnd; ruleIndex++ {
		for _, span := range spans {
			if strings.Contains(span.Text, cfg.Rules[ruleIndex].Term) {
				return ruleIndex, span.Role, true
			}
		}
	}
	return -1, "", false
}

func applyMode(spans []textSpan, cfg *configSnapshot) (*blockMatch, bool) {
	blockEnd, stripEnd := rulePhaseEnds(cfg)
	if blockEnd > 0 {
		if !cfg.IgnoreCase {
			ruleIndex, role, matched := matchExactBlock(spans, cfg)
			if matched {
				return &blockMatch{Term: cfg.Rules[ruleIndex].Term, Role: role}, false
			}
		} else {
			matcher := cfg.BlockMatcher
			if matcher == nil {
				matcher = newFoldMatcher(cfg.Rules[:blockEnd])
			}
			bestRule := -1
			bestRole := ""
			for _, span := range spans {
				ruleIndex, matched := matcher.match(span.Text)
				if matched && ruleIndex >= 0 && ruleIndex < blockEnd && (bestRule < 0 || ruleIndex < bestRule) {
					bestRule = ruleIndex
					bestRole = span.Role
					if bestRule == 0 {
						break
					}
				}
			}
			if bestRule >= 0 {
				return &blockMatch{Term: cfg.Rules[bestRule].Term, Role: bestRole}, false
			}
		}
	}

	retainOriginal := blockEnd < stripEnd && stripEnd < len(cfg.Rules)
	var original []string
	if retainOriginal {
		original = make([]string, len(spans))
		for i := range spans {
			original[i] = spans[i].Text
		}
	} else {
		for i := range spans {
			spans[i].Changed = false
		}
	}

	if cfg.IgnoreCase && cfg.RewriteMatcher != nil {
		for i := range spans {
			spans[i].SkipFoldRewrite = false
		}
		rewriteRuleCount := len(cfg.Rules) - blockEnd
		for i := range spans {
			if useFoldRewritePreflight(rewriteRuleCount, len(spans[i].Text)) {
				_, matched := cfg.RewriteMatcher.match(spans[i].Text)
				spans[i].SkipFoldRewrite = !matched
			}
		}
	}

	changed := false
	for _, rule := range cfg.Rules[blockEnd:stripEnd] {
		for i := range spans {
			if cfg.IgnoreCase && spans[i].SkipFoldRewrite {
				continue
			}
			text, matched := stripRule(spans[i].Text, rule, cfg.IgnoreCase)
			if matched {
				spans[i].Text = text
				if !retainOriginal {
					spans[i].Changed = true
					changed = true
				}
			}
		}
	}
	for _, rule := range cfg.Rules[stripEnd:] {
		for i := range spans {
			if cfg.IgnoreCase && spans[i].SkipFoldRewrite {
				continue
			}
			text, matched := obfuscateRule(spans[i].Text, rule, cfg.IgnoreCase, cfg.ObfsChar)
			if matched {
				spans[i].Text = text
				if !retainOriginal {
					spans[i].Changed = true
					changed = true
				}
			}
		}
	}
	if retainOriginal {
		for i := range spans {
			spans[i].Changed = spans[i].Text != original[i]
			changed = changed || spans[i].Changed
		}
	}
	return nil, changed
}

func rebuildBody(body []byte, spans []textSpan) ([]byte, error) {
	maxInt := int(^uint(0) >> 1)
	lowerBound := 0
	previousEnd := 0
	changed := false
	for _, span := range spans {
		if !span.Changed {
			continue
		}
		if span.RawStart < 0 || span.RawStart >= span.RawEnd || span.RawEnd > len(body) || span.RawStart < previousEnd {
			return nil, errInvalidSpan
		}
		unchangedLen := span.RawStart - previousEnd
		if lowerBound > maxInt-unchangedLen {
			return nil, errInvalidSpan
		}
		lowerBound += unchangedLen
		if lowerBound > maxInt-2 {
			return nil, errInvalidSpan
		}
		lowerBound += 2
		previousEnd = span.RawEnd
		changed = true
	}
	if !changed {
		return nil, nil
	}
	tailLen := len(body) - previousEnd
	if lowerBound > maxInt-tailLen {
		return nil, errInvalidSpan
	}
	lowerBound += tailLen

	var out bytes.Buffer
	out.Grow(lowerBound)
	encoder := json.NewEncoder(&out)
	previousEnd = 0
	for i := range spans {
		span := &spans[i]
		if !span.Changed {
			continue
		}
		out.Write(body[previousEnd:span.RawStart])
		if err := encoder.Encode(&span.Text); err != nil {
			return nil, err
		}
		if out.Len() == 0 || out.Bytes()[out.Len()-1] != '\n' {
			return nil, errors.New("JSON encoder output missing trailing newline")
		}
		out.Truncate(out.Len() - 1)
		previousEnd = span.RawEnd
	}
	out.Write(body[previousEnd:])
	return out.Bytes(), nil
}
