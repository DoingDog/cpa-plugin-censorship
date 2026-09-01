package main

import "errors"

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
	if cfg.Mode == modeBlock {
		for _, rule := range cfg.Rules {
			for i := range spans {
				if containsRule(spans[i].Text, rule, cfg.IgnoreCase) {
					return &blockMatch{Term: rule.Term, Role: spans[i].Role}, false
				}
			}
		}
	}
	return nil, false
}

func rebuildBody([]byte, []textSpan) ([]byte, error) {
	return nil, nil
}
