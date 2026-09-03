package main

import (
	"encoding/json"
	"errors"
)

type textSpan struct {
	RawStart int
	RawEnd   int
	Text     string
	Role     string
	Changed  bool
}

type blockMatch struct {
	Term string
	Role string
}

type transformResult struct {
	Body    []byte
	Blocked *blockMatch
	Invalid bool
}

func transformRequest(body []byte, sourceFormat string, cfg *configSnapshot) (transformResult, error) {
	if cfg == nil || len(cfg.Rules) == 0 || !cfg.Formats.has(sourceFormat) || len(cfg.Formats) == 0 || len(cfg.Roles) == 0 {
		return transformResult{}, nil
	}
	spans, err := selectTextSpans(body, sourceFormat, cfg.Roles)
	if errors.Is(err, errInvalidRequest) {
		return transformResult{Invalid: true}, nil
	}
	if err != nil {
		return transformResult{}, err
	}
	blocked, changed := applyMode(spans, cfg)
	if blocked != nil {
		return transformResult{Blocked: blocked}, nil
	}
	if !changed {
		return transformResult{}, nil
	}
	out, err := rebuildBody(body, spans)
	return transformResult{Body: out}, err
}

func applyMode(spans []textSpan, cfg *configSnapshot) (*blockMatch, bool) {
	switch cfg.Mode {
	case modeBlock:
		if !cfg.IgnoreCase {
			for _, rule := range cfg.Rules {
				for i := range spans {
					if containsRule(spans[i].Text, rule, false) {
						return &blockMatch{Term: rule.Term, Role: spans[i].Role}, false
					}
				}
			}
			break
		}
		matcher := cfg.BlockMatcher
		if matcher == nil {
			matcher = newFoldMatcher(cfg.Rules)
		}
		bestRule := -1
		bestRole := ""
		for _, span := range spans {
			ruleIndex, ok := matcher.match(span.Text)
			if ok && (bestRule < 0 || ruleIndex < bestRule) {
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
	case modeStrip:
		changed := false
		for _, rule := range cfg.Rules {
			for i := range spans {
				text, matched := stripRule(spans[i].Text, rule, cfg.IgnoreCase)
				if matched {
					spans[i].Text = text
					spans[i].Changed = true
					changed = true
				}
			}
		}
		return nil, changed
	case modeObfs:
		changed := false
		for _, rule := range cfg.Rules {
			for i := range spans {
				text, matched := obfuscateRule(spans[i].Text, rule, cfg.IgnoreCase, cfg.ObfsChar)
				if matched {
					spans[i].Text = text
					spans[i].Changed = true
					changed = true
				}
			}
		}
		return nil, changed
	}
	return nil, false
}

func rebuildBody(body []byte, spans []textSpan) ([]byte, error) {
	finalLen := len(body)
	var replacements [][]byte
	previousEnd := 0
	maxInt := int(^uint(0) >> 1)
	for i, span := range spans {
		if !span.Changed {
			continue
		}
		if span.RawStart < previousEnd || span.RawStart < 0 || span.RawStart >= span.RawEnd || span.RawEnd > len(body) {
			return nil, errInvalidSpan
		}
		replacement, err := json.Marshal(span.Text)
		if err != nil {
			return nil, err
		}
		if replacements == nil {
			replacements = make([][]byte, len(spans))
		}
		replacements[i] = replacement
		rawLen := span.RawEnd - span.RawStart
		if rawLen > finalLen {
			return nil, errInvalidSpan
		}
		finalLen -= rawLen
		if len(replacement) > maxInt-finalLen {
			return nil, errInvalidSpan
		}
		finalLen += len(replacement)
		previousEnd = span.RawEnd
	}
	if replacements == nil {
		return nil, nil
	}

	out := make([]byte, finalLen)
	source, destination := len(body), finalLen
	for i := len(spans) - 1; i >= 0; i-- {
		if !spans[i].Changed {
			continue
		}
		span := spans[i]
		tailLen := source - span.RawEnd
		if tailLen < 0 || tailLen > destination {
			return nil, errInvalidSpan
		}
		destination -= tailLen
		copy(out[destination:destination+tailLen], body[span.RawEnd:source])
		replacement := replacements[i]
		if len(replacement) > destination {
			return nil, errInvalidSpan
		}
		destination -= len(replacement)
		copy(out[destination:destination+len(replacement)], replacement)
		source = span.RawStart
	}
	if source > destination {
		return nil, errInvalidSpan
	}
	destination -= source
	copy(out[destination:destination+source], body[:source])
	source = 0
	if source != 0 || destination != 0 {
		return nil, errInvalidSpan
	}
	return out, nil
}
