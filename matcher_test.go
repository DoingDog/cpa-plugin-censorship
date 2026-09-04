package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFoldMatcherSelectsLowestRuleIndexAcrossOccurrences(t *testing.T) {
	rules := []compiledRule{
		{Term: "ab", Runes: []rune("ab")},
		{Term: "a", Runes: []rune("a")},
	}
	matcher := newFoldMatcher(rules)
	if got, ok := matcher.match("xABy"); !ok || got != 0 {
		t.Fatalf("matcher.match() = %d, %t; want rule index 0, true", got, ok)
	}
}

func TestFoldClassRuneUnifiesKelvinSign(t *testing.T) {
	if got, want := foldClassRune('K'), foldClassRune('K'); got != want {
		t.Fatalf("foldClassRune(K) = %U, foldClassRune(K) = %U; want equal keys", got, want)
	}
	if got, want := foldClassRune('K'), foldClassRune('k'); got != want {
		t.Fatalf("foldClassRune(K) = %U, foldClassRune(k) = %U; want equal keys", got, want)
	}
}

func TestFoldMatcherIncludesFailureSuffixRules(t *testing.T) {
	rules := []compiledRule{
		{Term: "bc", Runes: []rune("bc")},
		{Term: "abc", Runes: []rune("abc")},
	}
	matcher := newFoldMatcher(rules)
	if got, ok := matcher.match("ABC"); !ok || got != 0 {
		t.Fatalf("matcher.match() = %d, %t; want suffix rule index 0, true", got, ok)
	}
}

func TestFoldMatcherMatchesRuleMajorOracle(t *testing.T) {
	cases := []struct {
		name  string
		rules []string
		text  string
	}{
		{name: "prefix", rules: []string{"a", "aa"}, text: "cAA"},
		{name: "suffix", rules: []string{"bc", "abc"}, text: "zABC"},
		{name: "duplicate-folded", rules: []string{"K", "k"}, text: "K"},
		{name: "sigma", rules: []string{"Σ", "ς"}, text: "σ"},
		{name: "different-byte-length", rules: []string{"K"}, text: "K"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := make([]compiledRule, len(tc.rules))
			for i, term := range tc.rules {
				rules[i] = compiledRule{Term: term, Runes: []rune(term)}
			}
			want := -1
			for i, rule := range rules {
				if containsRule(tc.text, rule, true) {
					want = i
					break
				}
			}
			got, ok := newFoldMatcher(rules).match(tc.text)
			if got != want || ok != (want >= 0) {
				t.Fatalf("matcher.match() = %d, %t; want oracle %d, %t", got, ok, want, want >= 0)
			}
		})
	}
}

func TestFoldBlockMatcherDoesNotCrossSpans(t *testing.T) {
	rules := []compiledRule{{Term: "abc", Runes: []rune("abc")}}
	cfg := &configSnapshot{
		Mode:         modeBlock,
		IgnoreCase:   true,
		Rules:        rules,
		BlockMatcher: newFoldMatcher(rules),
	}
	spans := []textSpan{
		{Text: "ab", Role: "user"},
		{Text: "c", Role: "developer"},
	}
	blocked, changed := applyMode(spans, cfg)
	if changed || blocked != nil {
		t.Fatalf("applyMode() = %#v, %t; want no block across spans", blocked, changed)
	}
}

func TestFoldBlockMatcherPreservesRuleMajorDocumentRole(t *testing.T) {
	rules := []compiledRule{
		{Term: "ab", Runes: []rune("ab")},
		{Term: "a", Runes: []rune("a")},
	}
	cfg := &configSnapshot{
		Mode:         modeBlock,
		IgnoreCase:   true,
		Rules:        rules,
		BlockMatcher: newFoldMatcher(rules),
	}
	spans := []textSpan{
		{Text: "a", Role: "user"},
		{Text: "AB", Role: "developer"},
	}
	blocked, changed := applyMode(spans, cfg)
	if changed || blocked == nil {
		t.Fatalf("applyMode() = %#v, %t; want block", blocked, changed)
	}
	if blocked.Term != "ab" || blocked.Role != "developer" {
		t.Fatalf("block = %#v; want term ab and role developer", blocked)
	}
}

func TestFoldStripRuleProcessesAllNonOverlappingOccurrences(t *testing.T) {
	got, matched := stripFoldRule("aAaAA", []rune("aa"))
	if !matched || got != "A" {
		t.Fatalf("stripFoldRule() = %q, %t; want %q, true", got, matched, "A")
	}
}

func TestFoldObfuscateRulePreservesSourceCasePerOccurrence(t *testing.T) {
	const char = "⁠"
	got, matched := obfuscateFoldRule("aAaA", []rune("aa"), char)
	if !matched || got != "a"+char+"A"+"a"+char+"A" {
		t.Fatalf("obfuscateFoldRule() = %q, %t; want %q, true", got, matched, "a"+char+"A"+"a"+char+"A")
	}
}

func TestContainsRuleCaseModes(t *testing.T) {
	rule := compiledRule{Term: "Alpha", Runes: []rune("Alpha")}
	if containsRule("alpha", rule, false) {
		t.Fatal("default matching ignored case")
	}
	if !containsRule("xxaLPHAyy", rule, true) {
		t.Fatal("ignore_case did not match ASCII mixed case")
	}
	cases := []struct {
		text string
		term string
		want bool
	}{
		{text: "ς", term: "Σ", want: true},
		{text: "K", term: "K", want: true},
		{text: "STRASSE", term: "straße", want: false},
		{text: "é", term: "é", want: false},
	}
	for _, tc := range cases {
		r := compiledRule{Term: tc.term, Runes: []rune(tc.term)}
		if got := containsRule(tc.text, r, true); got != tc.want {
			t.Errorf("containsRule(%q, %q) = %t, want %t", tc.text, tc.term, got, tc.want)
		}
	}
}

func TestFoldMatcherASCIITransitionsSurviveClearedRootMap(t *testing.T) {
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: true,
		Rules:      []compiledRule{{Term: "ab"}},
	}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	matcher := cfg.BlockMatcher
	var root [utf8.RuneSelf]int = matcher.asciiRoot
	if root['a'] == 0 {
		t.Fatal("ASCII root table has no transition for a")
	}
	matcher.nodes[0].next = nil
	if got, ok := matcher.match("xxAB"); !ok || got != 0 {
		t.Fatalf("matcher.match() = %d, %t; want 0, true", got, ok)
	}
}

func TestFoldMatcherFailureToRootUsesASCIITable(t *testing.T) {
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: true,
		Rules: []compiledRule{
			{Term: "acx"},
			{Term: "b"},
		},
	}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	matcher := cfg.BlockMatcher
	matcher.nodes[0].next = nil
	if got, ok := matcher.match("acB"); !ok || got != 1 {
		t.Fatalf("matcher.match() after failure to root = %d, %t; want 1, true", got, ok)
	}
}

var (
	exactRewriteTextSink  string
	exactRewriteMatchSink bool
)

func TestRewriteExactReportsMatchesByLength(t *testing.T) {
	obfs := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: "éx"}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(obfs); err != nil {
		t.Fatal(err)
	}
	invalidTerm := string([]byte{0xff, 'x'})
	invalid := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: invalidTerm}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(invalid); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		text      string
		rule      compiledRule
		obfuscate bool
		want      string
		matched   bool
	}{
		{name: "strip hit", text: "aaaaa", rule: compiledRule{Term: "aa"}, want: "a", matched: true},
		{name: "strip miss", text: "plain", rule: compiledRule{Term: "aa"}, want: "plain"},
		{name: "obfs multibyte", text: "éxéx", rule: obfs.Rules[0], obfuscate: true, want: "é​xé​x", matched: true},
		{name: "obfs miss", text: "plain", rule: obfs.Rules[0], obfuscate: true, want: "plain"},
		{name: "obfs invalid UTF-8", text: "a" + invalidTerm, rule: invalid.Rules[0], obfuscate: true, want: "a" + string([]byte{0xff}) + "​x", matched: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, matched := rewriteExact(test.text, test.rule, test.obfuscate)
			if got != test.want || matched != test.matched {
				t.Fatalf("rewriteExact() = %q, %t; want %q, %t", got, matched, test.want, test.matched)
			}
		})
	}
}

func TestExactRewriteMissAllocatesNothing(t *testing.T) {
	obfs := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: "secret"}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(obfs); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		rule      compiledRule
		obfuscate bool
	}{
		{name: "strip", rule: compiledRule{Term: "secret"}},
		{name: "obfs", rule: obfs.Rules[0], obfuscate: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(1000, func() {
				exactRewriteTextSink, exactRewriteMatchSink = rewriteExact("plain text", test.rule, test.obfuscate)
			})
			if allocs != 0 {
				t.Fatalf("rewriteExact() miss allocations = %.1f, want 0", allocs)
			}
		})
	}
}

func TestExactObfuscationHitAllocationCeiling(t *testing.T) {
	cfg := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: "éx"}},
		ObfsChar: "⁠",
	}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("éx", 256)
	allocs := testing.AllocsPerRun(100, func() {
		exactRewriteTextSink, exactRewriteMatchSink = rewriteExact(text, cfg.Rules[0], true)
	})
	if allocs > 1 {
		t.Fatalf("rewriteExact() hit allocations = %.1f, want <= 1", allocs)
	}
	if !exactRewriteMatchSink || len(exactRewriteTextSink) != len(text)+256*len(cfg.ObfsChar) {
		t.Fatalf("rewriteExact() hit = len %d, %t", len(exactRewriteTextSink), exactRewriteMatchSink)
	}
}
