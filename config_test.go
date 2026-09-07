package main

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
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

func TestParseConfigYAMLWordsObject(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantTerms    []string
		wantBlockEnd int
		wantStripEnd int
	}{
		{name: "empty object", raw: "words: {}\n"},
		{name: "all empty buckets", raw: "words: {block: [], strip: [], obfs: []}\n"},
		{name: "block only", raw: "words: {block: [alpha]}\n", wantTerms: []string{"alpha"}, wantBlockEnd: 1, wantStripEnd: 1},
		{name: "strip only", raw: "words: {strip: [beta]}\n", wantTerms: []string{"beta"}, wantStripEnd: 1},
		{name: "obfs only", raw: "words: {obfs: [gamma]}\n", wantTerms: []string{"gamma"}},
		{name: "block and strip", raw: "words: {block: [alpha], strip: [beta]}\n", wantTerms: []string{"alpha", "beta"}, wantBlockEnd: 1, wantStripEnd: 2},
		{name: "block and obfs", raw: "words: {block: [alpha], obfs: [gamma]}\n", wantTerms: []string{"alpha", "gamma"}, wantBlockEnd: 1, wantStripEnd: 1},
		{name: "strip and obfs", raw: "words: {strip: [beta], obfs: [gamma]}\n", wantTerms: []string{"beta", "gamma"}, wantStripEnd: 1},
		{name: "all buckets", raw: "words: {block: [alpha], strip: [beta], obfs: [gamma]}\n", wantTerms: []string{"alpha", "beta", "gamma"}, wantBlockEnd: 1, wantStripEnd: 2},
		{name: "empty block", raw: "words: {block: [], strip: [beta]}\n", wantTerms: []string{"beta"}, wantStripEnd: 1},
		{name: "empty strip", raw: "words: {block: [alpha], strip: [], obfs: [gamma]}\n", wantTerms: []string{"alpha", "gamma"}, wantBlockEnd: 1, wantStripEnd: 1},
		{name: "empty obfs", raw: "words: {block: [alpha], obfs: []}\n", wantTerms: []string{"alpha"}, wantBlockEnd: 1, wantStripEnd: 1},
		{name: "preserves whitespace and duplicates", raw: "words: {block: [' ', alpha, alpha], strip: [alpha], obfs: [gamma, gamma]}\n", wantTerms: []string{" ", "alpha", "alpha", "alpha", "gamma", "gamma"}, wantBlockEnd: 3, wantStripEnd: 4},
		{name: "valid mode is ignored", raw: "mode: strip\nwords: {block: [alpha], strip: [beta], obfs: [gamma]}\n", wantTerms: []string{"alpha", "beta", "gamma"}, wantBlockEnd: 1, wantStripEnd: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := mustConfig(t, test.raw)
			if got := ruleTerms(cfg.Rules); !reflect.DeepEqual(got, test.wantTerms) {
				t.Fatalf("rules = %#v, want %#v", got, test.wantTerms)
			}
			if cfg.BlockEnd != test.wantBlockEnd || cfg.StripEnd != test.wantStripEnd {
				t.Fatalf("cut points = (%d, %d), want (%d, %d)", cfg.BlockEnd, cfg.StripEnd, test.wantBlockEnd, test.wantStripEnd)
			}
		})
	}
}

func TestParseConfigYAMLWordsObjectIgnoresMappingOrder(t *testing.T) {
	for _, raw := range []string{
		"words: {block: [alpha], strip: [beta], obfs: [gamma]}\n",
		"words: {block: [alpha], obfs: [gamma], strip: [beta]}\n",
		"words: {strip: [beta], block: [alpha], obfs: [gamma]}\n",
		"words: {strip: [beta], obfs: [gamma], block: [alpha]}\n",
		"words: {obfs: [gamma], block: [alpha], strip: [beta]}\n",
		"words: {obfs: [gamma], strip: [beta], block: [alpha]}\n",
	} {
		cfg := mustConfig(t, raw)
		if got, want := ruleTerms(cfg.Rules), []string{"alpha", "beta", "gamma"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("rules = %#v, want %#v", got, want)
		}
		if cfg.BlockEnd != 1 || cfg.StripEnd != 2 {
			t.Fatalf("cut points = (%d, %d), want (1, 2)", cfg.BlockEnd, cfg.StripEnd)
		}
	}
}

func TestParseConfigYAMLRejectsInvalidWordsObject(t *testing.T) {
	for _, raw := range []string{
		"words: null\n",
		"words: scalar\n",
		"words: {unknown: [x]}\n",
		"words:\n  block: [x]\n  block: [y]\n",
		"words: {block: null}\n",
		"words: {strip: [1]}\n",
		"words: {obfs: ['']}\n",
		"words: {block: scalar}\n",
		"words: {block: {nested: [x]}}\n",
		"words: {1: [x]}\n",
		"mode: nope\nwords: [alpha]\n",
		"mode: nope\nwords: {block: [alpha]}\n",
	} {
		if _, err := parseConfigYAML([]byte(raw)); err == nil {
			t.Errorf("parseConfigYAML(%q) error = nil", raw)
		}
	}
}

func TestParseConfigYAMLLegacyWordsUseFinalMode(t *testing.T) {
	strip := mustConfig(t, "words: [alpha]\nmode: strip\n")
	if got, want := ruleTerms(strip.Rules), []string{"alpha"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy strip rules = %#v, want %#v", got, want)
	}
	if strip.BlockEnd != 0 || strip.StripEnd != 1 {
		t.Fatalf("legacy strip cut points = (%d, %d), want (0, 1)", strip.BlockEnd, strip.StripEnd)
	}

	obfs := mustConfig(t, "words: [alpha]\nmode: obfs\n")
	if got, want := ruleTerms(obfs.Rules), []string{"alpha"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy obfs rules = %#v, want %#v", got, want)
	}
	if obfs.BlockEnd != 0 || obfs.StripEnd != 0 {
		t.Fatalf("legacy obfs cut points = (%d, %d), want (0, 0)", obfs.BlockEnd, obfs.StripEnd)
	}
}

func TestParseConfigYAMLValidatesOnlyEffectiveObfsTerms(t *testing.T) {
	if _, err := parseConfigYAML([]byte("words: {block: [x], strip: ['a​b'], obfs: [alpha]}\n")); err != nil {
		t.Fatalf("non-obfs terms rejected: %v", err)
	}
	for _, raw := range []string{
		"words: {block: [alpha], obfs: [x]}\n",
		"words: {block: [alpha], obfs: ['a​b']}\n",
		"words: [x]\nmode: obfs\n",
	} {
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

func ruleTerms(rules []compiledRule) []string {
	if rules == nil {
		return nil
	}
	terms := make([]string, len(rules))
	for i, rule := range rules {
		terms[i] = rule.Term
	}
	return terms
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

func TestConcurrentReconfigureObservesOnlyWholeSnapshot(t *testing.T) {
	const configA = "mode: strip\nwords: [alpha]\nscope:\n  formats: [openai]\n  roles: [user]\n"
	const configB = "mode: obfs\nignore_case: true\nwords: [Beta]\nscope:\n  formats: [openai]\n  roles: [user]\nobfs:\n  char: '⁠'\n"
	body := []byte(`{"messages":[{"role":"user","content":"alpha BETA"}]}`)
	wantA := []byte(`{"messages":[{"role":"user","content":" BETA"}]}`)
	wantB := []byte(`{"messages":[{"role":"user","content":"alpha B⁠ETA"}]}`)
	registerConfig(t, configA)

	done := make(chan struct{})
	started := make(chan struct{}, 32)
	errs := make(chan error, 32)
	var calls atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			first := true
			for {
				select {
				case <-done:
					return
				default:
				}
				resp, err := callIntercept("openai", body)
				if first {
					started <- struct{}{}
					first = false
				}
				if err != nil {
					errs <- err
					return
				}
				calls.Add(1)
				if !bytes.Equal(resp.Body, wantA) && !bytes.Equal(resp.Body, wantB) {
					errs <- fmt.Errorf("mixed snapshot body: %s", resp.Body)
					return
				}
			}
		}()
	}
	for i := 0; i < 32; i++ {
		<-started
	}
	before := calls.Load()
	for i := 0; i < 1000; i++ {
		if i%2 == 0 {
			reconfigureConfig(t, configB)
		} else {
			reconfigureConfig(t, configA)
		}
	}
	after := calls.Load()
	close(done)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("no interception completed during reconfiguration")
	}
}

func TestCompileSnapshotBuildsOnlyActiveDerivedData(t *testing.T) {
	tests := []struct {
		name               string
		raw                string
		wantRunes          bool
		wantBlockMatcher   bool
		wantRewriteMatcher bool
		wantExact          string
	}{
		{name: "exact block", raw: "mode: block\nwords: [K]\n"},
		{name: "exact strip", raw: "mode: strip\nwords: [K]\n"},
		{name: "exact obfs", raw: "mode: obfs\nwords: [éx]\n", wantExact: "é​x"},
		{name: "folded block", raw: "mode: block\nignore_case: true\nwords: [K]\n", wantRunes: true, wantBlockMatcher: true},
		{name: "folded strip small", raw: "mode: strip\nignore_case: true\nwords: [K]\n", wantRunes: true},
		{name: "folded strip preflight", raw: "mode: strip\nignore_case: true\nwords: [a, b, c, d, e, f, g, h]\n", wantRunes: true, wantRewriteMatcher: true},
		{name: "folded obfs", raw: "mode: obfs\nignore_case: true\nwords: [éx]\n", wantRunes: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := mustConfig(t, test.raw)
			if got := cfg.BlockMatcher != nil; got != test.wantBlockMatcher {
				t.Fatalf("BlockMatcher present = %t, want %t", got, test.wantBlockMatcher)
			}
			if got := cfg.RewriteMatcher != nil; got != test.wantRewriteMatcher {
				t.Fatalf("RewriteMatcher present = %t, want %t", got, test.wantRewriteMatcher)
			}
			for i, rule := range cfg.Rules {
				if got := rule.Runes != nil; got != test.wantRunes {
					t.Errorf("rule %d Runes present = %t, want %t", i, got, test.wantRunes)
				}
				if rule.FoldFailure != nil {
					t.Errorf("rule %d FoldFailure = %v, want nil", i, rule.FoldFailure)
				}
				wantExact := ""
				if i == 0 {
					wantExact = test.wantExact
				}
				if rule.ExactReplacement != wantExact {
					t.Errorf("rule %d ExactReplacement = %q, want %q", i, rule.ExactReplacement, wantExact)
				}
				if test.wantRunes {
					want := make([]rune, 0, len([]rune(rule.Term)))
					for _, r := range rule.Term {
						want = append(want, foldClassRune(r))
					}
					if !reflect.DeepEqual(rule.Runes, want) {
						t.Errorf("rule %d Runes = %U, want canonical %U", i, rule.Runes, want)
					}
				}
			}
		})
	}
}

func TestCompileSnapshotBuildsKMPOnlyForFoldedRewrite(t *testing.T) {
	tests := []struct {
		name       string
		mode       mode
		ignoreCase bool
		term       string
		want       []int
	}{
		{name: "exact block", mode: modeBlock, term: "ababaca"},
		{name: "exact strip", mode: modeStrip, term: "ababaca"},
		{name: "exact obfs", mode: modeObfs, term: "ababaca"},
		{name: "folded block", mode: modeBlock, ignoreCase: true, term: "ababaca"},
		{name: "folded strip short", mode: modeStrip, ignoreCase: true, term: "aba"},
		{name: "folded strip", mode: modeStrip, ignoreCase: true, term: "ababaca", want: []int{0, 0, 1, 2, 3, 0, 1}},
		{name: "folded obfs", mode: modeObfs, ignoreCase: true, term: "ababaca", want: []int{0, 0, 1, 2, 3, 0, 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &configSnapshot{
				Mode:       test.mode,
				IgnoreCase: test.ignoreCase,
				Rules:      []compiledRule{{Term: test.term}},
				ObfsChar:   "​",
			}
			if err := compileSnapshot(cfg); err != nil {
				t.Fatal(err)
			}
			if got := cfg.Rules[0].FoldFailure; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("FoldFailure = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCompileSnapshotValidatesObfsByRuneCount(t *testing.T) {
	if _, err := parseConfigYAML([]byte("mode: obfs\nwords: [é]\n")); err == nil {
		t.Fatal("single multibyte scalar obfs word was accepted")
	}
	cfg := mustConfig(t, "mode: obfs\nwords: [éx]\n")
	if cfg.Rules[0].Runes != nil {
		t.Fatalf("exact obfs Runes = %U, want nil", cfg.Rules[0].Runes)
	}
}

func TestSyntheticSnapshotsUseProductionCompiler(t *testing.T) {
	fuzzFold := snapshotForFuzz([]string{"K"}, 0, true)
	if fuzzFold == nil || fuzzFold.BlockMatcher == nil || !reflect.DeepEqual(fuzzFold.Rules[0].Runes, []rune{foldClassRune('K')}) {
		t.Fatalf("folded fuzz snapshot = %#v", fuzzFold)
	}

	fuzzExact := snapshotForFuzz([]string{"éx"}, 2, false)
	if fuzzExact == nil || fuzzExact.Rules[0].Runes != nil || fuzzExact.Rules[0].ExactReplacement != "é​x" {
		t.Fatalf("exact fuzz snapshot = %#v", fuzzExact)
	}

	benchmarkFold := benchmarkSnapshot(modeStrip, true, benchmarkRules(8, "K"))
	if benchmarkFold.RewriteMatcher == nil || !reflect.DeepEqual(benchmarkFold.Rules[7].Runes, []rune{foldClassRune('K')}) {
		t.Fatalf("folded benchmark snapshot = %#v", benchmarkFold)
	}

	_, benchmarkExact := benchmarkFixture(1<<10, 1, false)
	if benchmarkExact.BlockMatcher != nil || benchmarkExact.Rules[0].Runes != nil {
		t.Fatalf("exact benchmark snapshot = %#v", benchmarkExact)
	}
}

func TestCompileSnapshotPrecomputesExactReplacement(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nwords: [éx]\nobfs:\n  char: '⁠'\n")
	if got, want := cfg.Rules[0].ExactReplacement, "é⁠x"; got != want {
		t.Fatalf("ExactReplacement = %q, want %q", got, want)
	}

	folded := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [éx]\n")
	if got := folded.Rules[0].ExactReplacement; got != "" {
		t.Fatalf("folded ExactReplacement = %q, want empty", got)
	}

	term := string([]byte{0xff, 'x'})
	synthetic := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: term}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(synthetic); err != nil {
		t.Fatal(err)
	}
	want := string([]byte{0xff}) + "​x"
	if got := synthetic.Rules[0].ExactReplacement; got != want {
		t.Fatalf("invalid UTF-8 ExactReplacement bytes = % x, want % x", got, want)
	}
}
