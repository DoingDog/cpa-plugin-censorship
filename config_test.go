package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestParseConfigYAMLDefaultsAndValidation(t *testing.T) {
	defaults, err := parseConfigYAML(nil)
	if err != nil {
		t.Fatalf("parse defaults: %v", err)
	}
	if defaults.Mode != modeBlock || defaults.IgnoreCase || len(defaults.Rules) != 0 || defaults.ObfsChar != "​" {
		t.Fatalf("defaults = %#v", defaults)
	}
	if !reflect.DeepEqual(sortedKeys(defaults.Formats), []string{"claude", "gemini", "interactions", "openai", "openai-response"}) {
		t.Fatalf("formats = %v", sortedKeys(defaults.Formats))
	}
	if !reflect.DeepEqual(sortedKeys(defaults.Roles), []string{"developer", "system", "user"}) {
		t.Fatalf("roles = %v", sortedKeys(defaults.Roles))
	}

	invalid := []string{
		"[]\n",
		"scalar\n",
		"---\n{}\n---\n{}\n",
		"mode: nope\n",
		"mode: [block]\n",
		"mode: block\nmode: strip\n",
		"ignore_case: yes\n",
		"ignore_case: 'true'\n",
		"words: null\n",
		"words: ok\n",
		"words: [ok, '']\n",
		"words: [1]\n",
		"scope: []\n",
		"scope:\n  formats: openai\n",
		"scope:\n  formats: [OpenAI]\n",
		"scope:\n  formats: [openai]\n  formats: [claude]\n",
		"scope:\n  roles: {user: true}\n",
		"scope:\n  roles: [function]\n",
		"scope:\n  unknown: []\n",
		"obfs: []\n",
		"obfs:\n  char: [x]\n",
		"obfs:\n  char: x\n",
		"obfs:\n  char: '​'\n  char: '⁠'\n",
		"obfs:\n  unknown: x\n",
		"mode: obfs\nwords: [x]\n",
		"mode: obfs\nwords: ['a​b']\n",
		"word: [typo]\n",
	}
	for _, raw := range invalid {
		if _, err := parseConfigYAML([]byte(raw)); err == nil {
			t.Errorf("parseConfigYAML(%q) error = nil", raw)
		}
	}
}

func TestParseConfigPreservesRuleOrderWhitespaceAndDuplicates(t *testing.T) {
	cfg := mustConfig(t, "ignore_case: true\nwords: [' b ', a, a]\nscope:\n  formats: []\n  roles: []\n")
	got := []string{cfg.Rules[0].Term, cfg.Rules[1].Term, cfg.Rules[2].Term}
	if want := []string{" b ", "a", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %#v, want %#v", got, want)
	}
	if !cfg.IgnoreCase || len(cfg.Formats) != 0 || len(cfg.Roles) != 0 {
		t.Fatalf("snapshot = %#v", cfg)
	}

	duplicates := mustConfig(t, "words: [x]\nscope:\n  formats: [openai, openai]\n  roles: [user, user]\n")
	if len(duplicates.Formats) != 1 || len(duplicates.Roles) != 1 {
		t.Fatalf("duplicate scopes = %#v %#v", duplicates.Formats, duplicates.Roles)
	}
}

func sortedKeys(set scopeSet) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mustConfig(t *testing.T, raw string) *configSnapshot {
	t.Helper()
	cfg, err := parseConfigYAML([]byte(raw))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

func TestRegisterRejectsInvalidConfigAndAcceptsHostOwnedKeys(t *testing.T) {
	raw := lifecycleJSON(t, "ignore_case: yes\nwords: [x]\n")
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, raw), &env)
	if env.OK || env.Error == nil {
		t.Fatalf("invalid register envelope = %#v", env)
	}
	valid := "enabled: true\npriority: 7\nstore:\n  version: 1.2.3\nwords: [x]\n"
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, valid)), &env)
	if !env.OK {
		t.Fatalf("host-owned keys rejected: %#v", env)
	}
}
