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
		for _, rule := range cfg.Rules {
			for i := range spans {
				if containsRule(spans[i].Text, rule, cfg.IgnoreCase) {
					return &blockMatch{Term: rule.Term, Role: spans[i].Role}, false
				}
			}
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
	}
	return nil, false
}

func rebuildBody(body []byte, spans []textSpan) ([]byte, error) {
	out := make([]byte, 0, len(body))
	position := 0
	changed := false
	for _, span := range spans {
		if !span.Changed {
			continue
		}
		replacement, err := json.Marshal(span.Text)
		if err != nil {
			return nil, err
		}
		out = append(out, body[position:span.RawStart]...)
		out = append(out, replacement...)
		position = span.RawEnd
		changed = true
	}
	if !changed {
		return nil, nil
	}
	out = append(out, body[position:]...)
	return out, nil
}
