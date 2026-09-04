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
	Mode         mode
	IgnoreCase   bool
	Rules        []compiledRule
	Formats      scopeSet
	Roles        scopeSet
	ObfsChar     string
	BlockMatcher *foldMatcher
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
			if value.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("words must be a sequence")
			}
			cfg.Rules = make([]compiledRule, 0, len(value.Content))
			for _, item := range value.Content {
				term, err := stringScalar(item, "word")
				if err != nil {
					return nil, err
				}
				if term == "" {
					return nil, fmt.Errorf("word must not be empty")
				}
				cfg.Rules = append(cfg.Rules, compiledRule{Term: term})
			}
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

	if cfg.Mode == modeObfs {
		for _, rule := range cfg.Rules {
			if utf8.RuneCountInString(rule.Term) < 2 {
				return nil, fmt.Errorf("obfs word %q must contain at least two Unicode scalars", rule.Term)
			}
			if strings.Contains(rule.Term, cfg.ObfsChar) {
				return nil, fmt.Errorf("obfs word %q contains obfs.char", rule.Term)
			}
		}
	}
	if err := compileSnapshot(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func compileSnapshot(cfg *configSnapshot) error {
	cfg.BlockMatcher = nil
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		rule.Runes = nil
		rule.ExactReplacement = ""
		rule.FoldFailure = nil
	}

	if cfg.IgnoreCase {
		for i := range cfg.Rules {
			rule := &cfg.Rules[i]
			rule.Runes = make([]rune, 0, utf8.RuneCountInString(rule.Term))
			for _, r := range rule.Term {
				rule.Runes = append(rule.Runes, foldClassRune(r))
			}
		}
		switch cfg.Mode {
		case modeBlock:
			cfg.BlockMatcher = newFoldMatcher(cfg.Rules)
		case modeStrip, modeObfs:
			if len(cfg.Rules) >= foldRewritePreflightMinRules {
				cfg.BlockMatcher = newFoldMatcher(cfg.Rules)
			}
		}
		return nil
	}

	if cfg.Mode == modeObfs {
		for i := range cfg.Rules {
			rule := &cfg.Rules[i]
			_, firstSize := utf8.DecodeRuneInString(rule.Term)
			rule.ExactReplacement = rule.Term[:firstSize] + cfg.ObfsChar + rule.Term[firstSize:]
		}
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
