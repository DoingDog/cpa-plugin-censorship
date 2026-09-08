package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type mode string

const (
	modeBlock mode = "block"
	modeStrip mode = "strip"
	modeObfs  mode = "obfs"
)

type compiledRule struct {
	Term             string
	Runes            []rune
	ExactReplacement string
	FoldFailure      []int
}

type scopeSet map[string]struct{}

type configSnapshot struct {
	// Mode is retained until transform.go uses the per-action ranges directly.
	Mode              mode
	IgnoreCase        bool
	Rules             []compiledRule
	BlockEnd          int
	StripEnd          int
	Formats           scopeSet
	Roles             scopeSet
	ObfsChar          string
	BlockMatcher      *foldMatcher
	RewriteMatcher    *foldMatcher
	ExactBlockMatcher *byteMatcher
	rangesSet         bool
}

type parsedWords struct {
	legacy bool
	list   []string
	block  []string
	strip  []string
	obfs   []string
}

var activeConfig atomic.Pointer[configSnapshot]

func init() {
	activeConfig.Store(defaultSnapshot())
}

func defaultSnapshot() *configSnapshot {
	return &configSnapshot{
		Mode: modeBlock,
		Formats: scopeSet{
			"openai":          {},
			"openai-response": {},
			"claude":          {},
			"gemini":          {},
			"interactions":    {},
		},
		Roles: scopeSet{
			"system":    {},
			"developer": {},
			"user":      {},
		},
		ObfsChar: "​",
	}
}

func installSnapshot(next *configSnapshot) {
	activeConfig.Store(next)
}

func loadedSnapshot() *configSnapshot {
	return activeConfig.Load()
}

func (s scopeSet) has(value string) bool {
	_, ok := s[value]
	return ok
}

func parseConfigYAML(raw []byte) (*configSnapshot, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return defaultSnapshot(), nil
		}
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("config must contain at most one document")
		}
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("config must contain one document")
	}
	root := document.Content[0]
	if err := validateMapping(root, "config"); err != nil {
		return nil, err
	}

	cfg := defaultSnapshot()
	var words parsedWords
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i].Value, root.Content[i+1]
		switch key {
		case "mode":
			text, err := stringScalar(value, "mode")
			if err != nil {
				return nil, err
			}
			switch mode(text) {
			case modeBlock, modeStrip, modeObfs:
				cfg.Mode = mode(text)
			default:
				return nil, fmt.Errorf("invalid mode %q", text)
			}
		case "ignore_case":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				return nil, fmt.Errorf("ignore_case must be a boolean")
			}
			if err := value.Decode(&cfg.IgnoreCase); err != nil {
				return nil, fmt.Errorf("decode ignore_case: %w", err)
			}
		case "words":
			parsed, err := parseWords(value)
			if err != nil {
				return nil, err
			}
			words = parsed
		case "scope":
			if err := validateMapping(value, "scope"); err != nil {
				return nil, err
			}
			for j := 0; j < len(value.Content); j += 2 {
				scopeKey, scopeValue := value.Content[j].Value, value.Content[j+1]
				switch scopeKey {
				case "formats":
					formats, err := parseScopeSequence(scopeValue, "formats")
					if err != nil {
						return nil, err
					}
					cfg.Formats = formats
				case "roles":
					roles, err := parseScopeSequence(scopeValue, "roles")
					if err != nil {
						return nil, err
					}
					cfg.Roles = roles
				default:
					return nil, fmt.Errorf("unknown scope key %q", scopeKey)
				}
			}
		case "obfs":
			if err := validateMapping(value, "obfs"); err != nil {
				return nil, err
			}
			for j := 0; j < len(value.Content); j += 2 {
				obfsKey, obfsValue := value.Content[j].Value, value.Content[j+1]
				switch obfsKey {
				case "char":
					char, err := stringScalar(obfsValue, "obfs.char")
					if err != nil {
						return nil, err
					}
					if char != "​" && char != "⁠" {
						return nil, fmt.Errorf("obfs.char must be U+200B or U+2060")
					}
					cfg.ObfsChar = char
				default:
					return nil, fmt.Errorf("unknown obfs key %q", obfsKey)
				}
			}
		case "enabled", "priority", "store":
			// Host-owned keys do not belong to the plugin snapshot.
		default:
			return nil, fmt.Errorf("unknown config key %q", key)
		}
	}

	if words.legacy {
		switch cfg.Mode {
		case modeBlock:
			words.block = words.list
		case modeStrip:
			words.strip = words.list
		case modeObfs:
			words.obfs = words.list
		}
	}

	total := len(words.block) + len(words.strip) + len(words.obfs)
	if total > 0 {
		cfg.Rules = make([]compiledRule, 0, total)
	}
	for _, term := range words.block {
		cfg.Rules = append(cfg.Rules, compiledRule{Term: term})
	}
	cfg.BlockEnd = len(cfg.Rules)
	for _, term := range words.strip {
		cfg.Rules = append(cfg.Rules, compiledRule{Term: term})
	}
	cfg.StripEnd = len(cfg.Rules)
	for _, term := range words.obfs {
		cfg.Rules = append(cfg.Rules, compiledRule{Term: term})
	}
	cfg.rangesSet = true

	for _, rule := range cfg.Rules[cfg.StripEnd:] {
		if utf8.RuneCountInString(rule.Term) < 2 {
			return nil, fmt.Errorf("obfs word %q must contain at least two Unicode scalars", rule.Term)
		}
		if strings.Contains(rule.Term, cfg.ObfsChar) {
			return nil, fmt.Errorf("obfs word %q contains obfs.char", rule.Term)
		}
	}
	if err := compileSnapshot(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseWords(node *yaml.Node) (parsedWords, error) {
	switch node.Kind {
	case yaml.SequenceNode:
		list, err := parseWordSequence(node, "words")
		if err != nil {
			return parsedWords{}, err
		}
		return parsedWords{legacy: true, list: list}, nil
	case yaml.MappingNode:
		if err := validateMapping(node, "words"); err != nil {
			return parsedWords{}, err
		}
		var words parsedWords
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			terms, err := parseWordSequence(value, "words."+key)
			if err != nil {
				return parsedWords{}, err
			}
			switch key {
			case "block":
				words.block = terms
			case "strip":
				words.strip = terms
			case "obfs":
				words.obfs = terms
			default:
				return parsedWords{}, fmt.Errorf("unknown words key %q", key)
			}
		}
		return words, nil
	default:
		return parsedWords{}, fmt.Errorf("words must be a sequence or mapping")
	}
}

func parseWordSequence(node *yaml.Node, name string) ([]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a sequence", name)
	}
	if len(node.Content) == 0 {
		return nil, nil
	}
	terms := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		term, err := stringScalar(item, "word")
		if err != nil {
			return nil, err
		}
		if term == "" {
			return nil, fmt.Errorf("word must not be empty")
		}
		terms = append(terms, term)
	}
	return terms, nil
}

func compileSnapshot(cfg *configSnapshot) error {
	cfg.BlockMatcher = nil
	cfg.RewriteMatcher = nil
	cfg.ExactBlockMatcher = nil
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		rule.Runes = nil
		rule.ExactReplacement = ""
		rule.FoldFailure = nil
	}

	blockEnd, stripEnd := cfg.BlockEnd, cfg.StripEnd
	if !cfg.rangesSet {
		switch cfg.Mode {
		case modeBlock:
			blockEnd, stripEnd = len(cfg.Rules), len(cfg.Rules)
		case modeStrip:
			stripEnd = len(cfg.Rules)
		}
	}
	blockRules := cfg.Rules[:blockEnd]
	rewriteRules := cfg.Rules[blockEnd:]
	obfsRules := cfg.Rules[stripEnd:]

	if cfg.IgnoreCase {
		for i := range cfg.Rules {
			rule := &cfg.Rules[i]
			rule.Runes = make([]rune, 0, utf8.RuneCountInString(rule.Term))
			for _, r := range rule.Term {
				rule.Runes = append(rule.Runes, foldClassRune(r))
			}
		}
		if len(blockRules) > 0 {
			cfg.BlockMatcher = newFoldMatcher(blockRules)
		}
		if len(rewriteRules) >= foldRewritePreflightMinRules {
			cfg.RewriteMatcher = newFoldMatcher(rewriteRules)
		}
		for i := range rewriteRules {
			rule := &rewriteRules[i]
			if len(rule.Runes) >= foldKMPMinPatternScalars {
				rule.FoldFailure = buildFoldFailure(rule.Runes)
			}
		}
		return nil
	}

	if len(blockRules) >= exactByteMatcherMinRules {
		cfg.ExactBlockMatcher = newByteMatcher(blockRules[exactByteMatcherPrefixRules:], exactByteMatcherPrefixRules)
	}

	for i := range obfsRules {
		rule := &obfsRules[i]
		_, firstSize := utf8.DecodeRuneInString(rule.Term)
		rule.ExactReplacement = rule.Term[:firstSize] + cfg.ObfsChar + rule.Term[firstSize:]
	}
	return nil
}

func validateMapping(node *yaml.Node, name string) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a mapping", name)
	}
	seen := make(map[string]struct{}, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return fmt.Errorf("%s keys must be strings", name)
		}
		if _, exists := seen[key.Value]; exists {
			return fmt.Errorf("duplicate %s key %q", name, key.Value)
		}
		seen[key.Value] = struct{}{}
	}
	return nil
}

func stringScalar(node *yaml.Node, name string) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return node.Value, nil
}

func parseScopeSequence(node *yaml.Node, name string) (scopeSet, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("scope.%s must be a sequence", name)
	}
	out := make(scopeSet, len(node.Content))
	for _, item := range node.Content {
		value, err := stringScalar(item, "scope."+name+" value")
		if err != nil {
			return nil, err
		}
		allowed := false
		switch name {
		case "formats":
			switch value {
			case "openai", "openai-response", "claude", "gemini", "interactions":
				allowed = true
			}
		case "roles":
			switch value {
			case "system", "developer", "user", "assistant", "tool":
				allowed = true
			}
		}
		if !allowed {
			return nil, fmt.Errorf("invalid scope.%s value %q", name, value)
		}
		out[value] = struct{}{}
	}
	return out, nil
}
