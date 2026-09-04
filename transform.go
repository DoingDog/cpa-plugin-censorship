package main

import (
	"bytes"
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
